<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import ImageEditor from '$lib/ImageEditor.svelte';
	import { uploadPhoto, renderPreviewUrl, boothPrint, type EditState } from '$lib/api';

	interface Props {
		projectId: number;
		projectName: string;
	}

	let { projectId, projectName }: Props = $props();

	type BoothState = 'viewfinder' | 'countdown' | 'uploading' | 'editing' | 'previewing' | 'printing' | 'done';
	let state = $state<BoothState>('viewfinder');
	let photoId = $state(0);
	let countdown = $state(0); // user-selectable: 0, 3, or 5
	let countdownValue = $state(0);
	let error = $state('');

	let videoEl: HTMLVideoElement;
	let captureCanvas: HTMLCanvasElement;
	let stream: MediaStream | null = null;
	let facingMode = $state<'environment' | 'user'>('environment');

	// Pinch-to-zoom state
	let currentZoom = 1;
	let minZoom = 1;
	let maxZoom = 1;
	let lastPinchDist = 0;

	// Saved edits from the editor (for booth-print call)
	let savedEdits: EditState | null = null;

	let boothEl: HTMLDivElement;

	onMount(() => {
		startCamera();
		setBoothHeight();
		window.addEventListener('resize', setBoothHeight);
		return () => window.removeEventListener('resize', setBoothHeight);
	});

	function setBoothHeight() {
		if (boothEl) {
			boothEl.style.height = `${window.innerHeight}px`;
		}
	}

	onDestroy(() => {
		stopCamera();
	});

	async function startCamera() {
		try {
			stream = await navigator.mediaDevices.getUserMedia({
				video: {
					facingMode: facingMode,
					width: { ideal: 1920 },
					height: { ideal: 1080 }
				},
				audio: false
			});
			if (videoEl) {
				videoEl.srcObject = stream;
			}

			// Detect zoom capability
			const track = stream.getVideoTracks()[0];
			if (track) {
				const caps = track.getCapabilities?.() as any;
				if (caps?.zoom) {
					minZoom = caps.zoom.min || 1;
					maxZoom = caps.zoom.max || 1;
					currentZoom = (track.getSettings() as any).zoom || 1;
				}
			}
		} catch (e) {
			error = 'Camera access denied. Please allow camera permissions.';
		}
	}

	function applyZoom(zoom: number) {
		if (!stream || maxZoom <= minZoom) return;
		const track = stream.getVideoTracks()[0];
		if (!track) return;
		const clamped = Math.max(minZoom, Math.min(maxZoom, zoom));
		try {
			(track as any).applyConstraints({ advanced: [{ zoom: clamped } as any] });
			currentZoom = clamped;
		} catch { /* zoom not supported */ }
	}

	function handleTouchStart(e: TouchEvent) {
		if (e.touches.length === 2) {
			lastPinchDist = Math.hypot(
				e.touches[0].clientX - e.touches[1].clientX,
				e.touches[0].clientY - e.touches[1].clientY
			);
		}
	}

	function handleTouchMove(e: TouchEvent) {
		if (e.touches.length !== 2 || maxZoom <= minZoom) return;
		e.preventDefault();

		const dist = Math.hypot(
			e.touches[0].clientX - e.touches[1].clientX,
			e.touches[0].clientY - e.touches[1].clientY
		);

		if (lastPinchDist > 0) {
			const scale = dist / lastPinchDist;
			applyZoom(currentZoom * scale);
		}
		lastPinchDist = dist;
	}

	function handleTouchEnd() {
		lastPinchDist = 0;
	}

	let focusIndicator = $state<{ x: number; y: number; show: boolean }>({ x: 0, y: 0, show: false });
	let focusTimeout: ReturnType<typeof setTimeout>;

	function handleTapToFocus(e: MouseEvent | TouchEvent) {
		if (!stream || !videoEl) return;

		const track = stream.getVideoTracks()[0];
		if (!track) return;

		const caps = track.getCapabilities?.() as any;
		if (!caps?.focusMode?.includes('manual') && !caps?.focusMode?.includes('single-shot')) return;

		// Get tap position relative to video
		const rect = videoEl.getBoundingClientRect();
		let clientX: number, clientY: number;
		if ('touches' in e && e.touches.length === 1) {
			clientX = e.touches[0].clientX;
			clientY = e.touches[0].clientY;
		} else if ('clientX' in e) {
			clientX = e.clientX;
			clientY = e.clientY;
		} else {
			return;
		}

		const x = (clientX - rect.left) / rect.width;
		const y = (clientY - rect.top) / rect.height;

		// Show focus indicator
		focusIndicator = { x: clientX, y: clientY, show: true };
		clearTimeout(focusTimeout);
		focusTimeout = setTimeout(() => focusIndicator = { ...focusIndicator, show: false }, 1000);

		// Apply focus point
		try {
			const constraints: any = { advanced: [{ focusMode: 'single-shot' }] };
			if (caps?.pointsOfInterest) {
				constraints.advanced[0].pointsOfInterest = [{ x, y }];
			}
			track.applyConstraints(constraints);
		} catch { /* focus not supported */ }
	}

	function stopCamera() {
		if (stream) {
			stream.getTracks().forEach(t => t.stop());
			stream = null;
		}
	}

	async function switchCamera() {
		stopCamera();
		facingMode = facingMode === 'environment' ? 'user' : 'environment';
		await startCamera();
	}

	function takePhoto() {
		if (countdown > 0) {
			state = 'countdown';
			countdownValue = countdown;
			runCountdown();
		} else {
			capture();
		}
	}

	function runCountdown() {
		if (countdownValue <= 0) {
			capture();
			return;
		}
		setTimeout(() => {
			countdownValue--;
			runCountdown();
		}, 1000);
	}

	async function capture() {
		if (!videoEl || !captureCanvas) return;

		const vw = videoEl.videoWidth;
		const vh = videoEl.videoHeight;
		captureCanvas.width = vw;
		captureCanvas.height = vh;

		const ctx = captureCanvas.getContext('2d')!;

		// Mirror if using front camera
		if (facingMode === 'user') {
			ctx.translate(vw, 0);
			ctx.scale(-1, 1);
		}

		ctx.drawImage(videoEl, 0, 0, vw, vh);

		state = 'uploading';

		try {
			const blob = await new Promise<Blob>((resolve, reject) => {
				captureCanvas.toBlob(b => b ? resolve(b) : reject('Failed to capture'), 'image/jpeg', 0.92);
			});

			const file = new File([blob], 'booth-capture.jpg', { type: 'image/jpeg' });
			const result = await uploadPhoto(file, projectId);
			photoId = result.id;

			// Wait for preview to be ready
			for (let i = 0; i < 30; i++) {
				const res = await fetch(`/api/photos/${photoId}/preview`, { method: 'HEAD' });
				if (res.ok) break;
				await new Promise(r => setTimeout(r, 1000));
			}

			state = 'editing';
		} catch (e) {
			error = 'Failed to capture photo. Please try again.';
			state = 'viewfinder';
		}
	}

	function handleEditorSave() {
		state = 'previewing';
	}

	async function handlePrint() {
		state = 'printing';
		try {
			// Build edits from whatever was last saved by the editor
			const edits: EditState = savedEdits || {
				crop_x: 0, crop_y: 0, crop_width: 1, crop_height: 1,
				rotation: 0, brightness: null, contrast: null, saturation: null,
				overlay_overrides: [], text_overrides: [], copies: 1
			};

			await boothPrint(photoId, edits);
			state = 'done';
		} catch (e) {
			error = 'Print failed. Please try again.';
			state = 'previewing';
		}
	}

	async function retake() {
		// Delete the captured photo if it exists
		if (photoId > 0) {
			try {
				const { deleteOwnPhoto } = await import('$lib/api');
				await deleteOwnPhoto(photoId);
			} catch { /* ignore — photo may not exist or already deleted */ }
		}
		photoId = 0;
		savedEdits = null;
		error = '';
		state = 'viewfinder';
		startCamera();
	}

	function takeAnother() {
		retake();
	}
</script>

<div class="booth" bind:this={boothEl}>
	{#if error}
		<div class="booth-error">
			<p>{error}</p>
			<button class="booth-btn" onclick={() => { error = ''; state = 'viewfinder'; }}>Try Again</button>
		</div>
	{/if}

	{#if state === 'viewfinder' || state === 'countdown'}
		<div class="viewfinder">
			<!-- Top bar -->
			<div class="viewfinder-top">
				<a href="/" class="booth-nav-btn">&larr; Back</a>
				<span class="booth-title">{projectName}</span>
				<div style="width: 60px;"></div>
			</div>

			<!-- svelte-ignore a11y_media_has_caption -->
			<div class="video-wrap"
				ontouchstart={handleTouchStart}
				ontouchmove={handleTouchMove}
				ontouchend={handleTouchEnd}
				onclick={handleTapToFocus}
			>
				<video bind:this={videoEl} autoplay playsinline class:mirror={facingMode === 'user'}></video>
				{#if focusIndicator.show}
					<div class="focus-ring" style="left: {focusIndicator.x}px; top: {focusIndicator.y}px;"></div>
				{/if}
			</div>

			{#if state === 'countdown'}
				<div class="countdown-overlay">
					<span class="countdown-num">{countdownValue}</span>
				</div>
			{/if}

			<div class="viewfinder-controls">
				<div class="vc-left">
					<select class="timer-select" bind:value={countdown}>
						<option value={0}>No timer</option>
						<option value={3}>3s</option>
						<option value={5}>5s</option>
					</select>
				</div>
				<div class="vc-center">
					<button class="capture-btn" onclick={takePhoto} disabled={state === 'countdown'}>
						<svg viewBox="0 0 24 24" width="28" height="28" fill="white" stroke="none">
							<path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z"/>
							<circle cx="12" cy="13" r="4"/>
						</svg>
					</button>
				</div>
				<div class="vc-right">
					<button class="booth-btn secondary" onclick={switchCamera}>
						<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M23 4v6h-6"/><path d="M1 20v-6h6"/>
							<path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/>
					</svg>
				</button>
			</div>
		</div>
	</div>

	{:else if state === 'uploading'}
		<div class="booth-center">
			<div class="spinner-lg"></div>
			<p>Processing...</p>
		</div>

	{:else if state === 'editing'}
		<div class="booth-editor">
			<div class="booth-header">
				<button class="booth-nav-btn" onclick={retake}>&larr; Retake</button>
				<h3>Edit Photo</h3>
				<div style="width: 80px;"></div>
			</div>
			<div class="booth-editor-content">
				<ImageEditor
					photoId={photoId}
					onSave={handleEditorSave}
				/>
			</div>
		</div>

	{:else if state === 'previewing'}
		<div class="booth-preview">
			<div class="booth-header">
				<button class="booth-nav-btn" onclick={retake}>&larr; Retake</button>
				<h3>Preview</h3>
				<div style="width: 80px;"></div>
			</div>
			<div class="preview-image">
				<img src={renderPreviewUrl(photoId) + '?t=' + Date.now()} alt="Preview" />
			</div>
			<div class="preview-controls">
				<button class="booth-btn edit-btn" onclick={() => state = 'editing'}>Edit</button>
				<button class="booth-btn print-btn" onclick={handlePrint}>Print</button>
			</div>
		</div>

	{:else if state === 'printing'}
		<div class="booth-center">
			<div class="spinner-lg"></div>
			<p>Sending to printer...</p>
		</div>

	{:else if state === 'done'}
		<div class="booth-center done">
			<svg class="done-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M22 11.08V12a10 10 0 11-5.93-9.14"/>
				<polyline points="22 4 12 14.01 9 11.01"/>
			</svg>
			<h2>Photo is printing!</h2>
			<p>{projectName}</p>
			<button class="booth-btn" onclick={takeAnother}>Take Another Photo</button>
		</div>
	{/if}
</div>

<canvas bind:this={captureCanvas} hidden></canvas>

<style>
	.booth {
		height: 100vh; /* fallback */
		height: 100dvh; /* modern browsers */
		/* JS override via setBoothHeight() uses window.innerHeight for Android */
		background: #000;
		color: #fff;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.booth-error {
		position: fixed; top: 0; left: 0; right: 0;
		background: rgba(248, 113, 113, 0.95);
		padding: 16px; text-align: center; z-index: 10;
	}

	/* Header bar for edit/preview screens */
	.booth-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 16px;
		background: #111;
		border-bottom: 1px solid #222;
		flex-shrink: 0;
	}

	.booth-header h3 {
		font-size: 1rem;
		font-weight: 600;
	}

	.booth-nav-btn {
		background: none;
		color: var(--accent);
		border: none;
		font-size: 0.9rem;
		padding: 8px 12px;
		min-height: auto;
		min-width: auto;
		cursor: pointer;
	}

	/* Viewfinder — fits within screen */
	.viewfinder {
		flex: 1;
		display: flex;
		flex-direction: column;
		position: relative;
		max-height: 100dvh;
	}

	.viewfinder-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 8px 12px;
		background: #111;
		flex-shrink: 0;
	}

	.booth-title {
		font-size: 0.85rem;
		color: #888;
		font-weight: 500;
	}

	.timer-select {
		background: #222;
		color: #ccc;
		border: 1px solid #444;
		border-radius: 6px;
		padding: 4px 8px;
		font-size: 0.8rem;
		min-height: auto;
		width: auto;
	}

	.video-wrap {
		flex: 1;
		position: relative;
		overflow: hidden;
		background: #111;
		min-height: 0;
		touch-action: none;
	}

	video {
		width: 100%;
		height: 100%;
		object-fit: contain;
		background: #111;
	}

	video.mirror { transform: scaleX(-1); }

	.focus-ring {
		position: fixed;
		width: 60px;
		height: 60px;
		border: 2px solid rgba(255, 255, 255, 0.8);
		border-radius: 50%;
		transform: translate(-50%, -50%);
		pointer-events: none;
		animation: focus-pulse 0.6s ease-out;
	}

	@keyframes focus-pulse {
		0% { transform: translate(-50%, -50%) scale(1.5); opacity: 0; }
		50% { opacity: 1; }
		100% { transform: translate(-50%, -50%) scale(1); opacity: 0.6; }
	}

	.viewfinder-controls {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 16px 24px;
		background: #111;
		flex-shrink: 0;
	}

	.vc-left, .vc-center, .vc-right {
		flex: 1;
		display: flex;
		align-items: center;
	}

	.vc-left { justify-content: flex-start; }
	.vc-center { justify-content: center; }
	.vc-right { justify-content: flex-end; }

	.capture-btn {
		width: 68px; height: 68px;
		border-radius: 50%;
		background: #e53e3e;
		border: 4px solid #ff6b6b;
		padding: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: auto;
		min-width: auto;
		cursor: pointer;
		transition: transform 0.1s;
		box-shadow: 0 2px 12px rgba(229, 62, 62, 0.4);
	}

	.capture-btn:active { transform: scale(0.9); background: #c53030; }
	.capture-btn:disabled { opacity: 0.5; }

	/* Countdown */
	.countdown-overlay {
		position: absolute; inset: 0;
		display: flex; align-items: center; justify-content: center;
		background: rgba(0,0,0,0.4);
	}

	.countdown-num {
		font-size: 8rem; font-weight: 900; color: white;
		text-shadow: 0 4px 24px rgba(0,0,0,0.5);
		animation: pulse 1s ease-in-out;
	}

	@keyframes pulse {
		0% { transform: scale(1.5); opacity: 0; }
		50% { opacity: 1; }
		100% { transform: scale(1); opacity: 1; }
	}

	/* Booth buttons */
	.booth-btn {
		padding: 14px 28px;
		font-size: 1rem;
		font-weight: 600;
		border-radius: 12px;
		background: var(--accent);
		color: white;
		border: none;
		min-height: 52px;
		cursor: pointer;
	}

	.booth-btn.secondary {
		background: rgba(255,255,255,0.15);
		min-height: auto;
		padding: 10px;
		border-radius: 50%;
		width: 48px; height: 48px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.print-btn { background: var(--success); color: #000; flex: 1; }
	.edit-btn { background: rgba(255,255,255,0.15); }

	/* Center states */
	.booth-center {
		flex: 1;
		display: flex; flex-direction: column;
		align-items: center; justify-content: center;
		gap: 16px; padding: 32px;
	}

	.booth-center p { color: #999; font-size: 1.1rem; }

	.spinner-lg {
		width: 60px; height: 60px;
		border: 4px solid #333;
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.done { gap: 20px; }
	.done-icon { width: 80px; height: 80px; color: var(--success); }
	.done h2 { font-size: 1.75rem; }

	/* Editor */
	.booth-editor {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.booth-editor-content {
		flex: 1;
		overflow-y: auto;
		padding: 16px;
		background: var(--bg);
	}

	/* Preview */
	.booth-preview {
		flex: 1;
		display: flex;
		flex-direction: column;
	}

	.preview-image {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		background: #000;
		padding: 16px;
		overflow: hidden;
	}

	.preview-image img {
		max-width: 100%;
		max-height: 60vh;
		object-fit: contain;
		border-radius: 8px;
	}

	.preview-controls {
		display: flex;
		gap: 12px;
		padding: 16px;
		background: #111;
		flex-shrink: 0;
	}
</style>
