<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getEdits, saveEdits, saveTransform, previewUrl, renderPreviewUrl, type EditsResponse, type EditState, type CropTransform } from '$lib/api';

	interface Props {
		photoId: number;
		onSave: () => void;
	}

	let { photoId, onSave }: Props = $props();

	let containerEl: HTMLDivElement;
	let canvasEl: HTMLCanvasElement;
	let fabricCanvas: any;
	let fabricImage: any;
	let cropRect: any;

	let activeTab = $state<'crop' | 'adjust'>('crop');
	let loading = $state(true);
	let canvasReady = $state(false);
	let saving = $state(false);
	let renderSrc = $state('');
	let renderLoading = $state(false);

	// CSS filter for real-time adjustment preview
	// Maps our -1..1 range to CSS filter values:
	//   brightness: 0 → 1.0 (no change), -1 → 0, 1 → 2
	//   contrast:   0 → 1.0, -1 → 0, 1 → 2
	//   saturate:   0 → 1.0, -1 → 0, 1 → 2
	let cssFilter = $derived(() => {
		const b = (brightness ?? projectBrightness) + 1; // 0..2
		const c = (contrast ?? projectContrast) + 1;
		const s = (saturation ?? projectSaturation) + 1;
		return `brightness(${b.toFixed(2)}) contrast(${c.toFixed(2)}) saturate(${s.toFixed(2)})`;
	});

	// Image state on canvas
	let cw = 0, ch = 0;
	let imgLeft = 0, imgTop = 0, imgW = 0, imgH = 0;

	// Editable state
	let cropX = $state(0);
	let cropY = $state(0);
	let cropWidth = $state(1);
	let cropHeight = $state(1);
	let rotation = $state(0);
	let orientation = $state<'landscape' | 'portrait'>('landscape');

	let brightness = $state<number | null>(null);
	let contrast = $state<number | null>(null);
	let saturation = $state<number | null>(null);
	let copies = $state(1);

	// Project defaults (shown as reference)
	let projectBrightness = $state(0);
	let projectContrast = $state(0);
	let projectSaturation = $state(0);

	const CROP_ASPECT_L = 3 / 2;
	const CROP_ASPECT_P = 2 / 3;

	function cropAspect(): number {
		return orientation === 'landscape' ? CROP_ASPECT_L : CROP_ASPECT_P;
	}

	let savedEdits: EditsResponse | null = null;

	onMount(async () => {
		// Phase 1: Load data (DOM not needed yet)
		const fabric = await import('fabric');
		(window as any).__fabric = fabric;

		try {
			savedEdits = await getEdits(photoId);
		} catch { /* no saved edits */ }

		// Restore non-canvas state from saved edits
		if (savedEdits?.overrides) {
			brightness = savedEdits.overrides.brightness;
			contrast = savedEdits.overrides.contrast;
			saturation = savedEdits.overrides.saturation;
		}

		if (savedEdits?.project) {
			projectBrightness = savedEdits.project.brightness;
			projectContrast = savedEdits.project.contrast;
			projectSaturation = savedEdits.project.saturation;
		}

		copies = savedEdits?.copies || 1;

		if (savedEdits?.transform) {
			cropX = savedEdits.transform.crop_x;
			cropY = savedEdits.transform.crop_y;
			cropWidth = savedEdits.transform.crop_width;
			cropHeight = savedEdits.transform.crop_height;
			rotation = savedEdits.transform.rotation;
		}

		// Phase 2: Show the editor DOM, then init canvas after it renders
		loading = false;

		// Wait for DOM to render the canvas element
		await new Promise(r => setTimeout(r, 100));

		initCanvas(fabric);
	});

	async function initCanvas(fabric: any) {
		if (!containerEl || !canvasEl) return;

		const maxW = Math.min(window.innerWidth - 32, 480);
		// Reserve space for controls (~250px for tabs, sliders, buttons)
		const maxH = Math.min(window.innerHeight - 280, 400);
		cw = Math.min(containerEl.clientWidth || maxW, maxW);

		// Load image first to determine canvas aspect ratio
		const img = await fabric.FabricImage.fromURL(previewUrl(photoId), { crossOrigin: 'anonymous' });
		fabricImage = img;

		const iw = img.width || 1;
		const ih = img.height || 1;
		const isSwapped = rotation === 90 || rotation === 270;
		const ew = isSwapped ? ih : iw;
		const eh = isSwapped ? iw : ih;
		const imageAspect = ew / eh;

		ch = Math.round(cw / imageAspect);
		ch = Math.max(ch, 150);
		ch = Math.min(ch, maxH);
		// If height was clamped, adjust width to maintain aspect ratio
		if (ch < Math.round(cw / imageAspect)) {
			cw = Math.round(ch * imageAspect);
		}

		fabricCanvas = new fabric.Canvas(canvasEl, {
			width: cw, height: ch,
			backgroundColor: '#000',
			selection: false
		});

		// Detect orientation (reuse iw/ih from above)
		if (savedEdits?.transform && savedEdits.transform.crop_width > 0 && savedEdits.transform.crop_height > 0) {
			const savedAspect = (savedEdits.transform.crop_width * iw) / (savedEdits.transform.crop_height * ih);
			orientation = savedAspect >= 1 ? 'landscape' : 'portrait';
		} else {
			orientation = iw >= ih ? 'landscape' : 'portrait';
		}

		layoutImage();
		fabricCanvas.add(fabricImage);
		createCropRect(true);
		fabricCanvas.on('mouse:dblclick', () => maximizeCrop());
		fabricCanvas.renderAll();
		canvasReady = true;
	}

	function layoutImage() {
		if (!fabricImage) return;
		const iw = fabricImage.width || 1;
		const ih = fabricImage.height || 1;

		const isSwapped = rotation === 90 || rotation === 270;
		const ew = isSwapped ? ih : iw;
		const eh = isSwapped ? iw : ih;

		// Fit image within canvas (show the full image, no cropping)
		const scale = Math.min(cw / ew, ch / eh);
		imgW = ew * scale;
		imgH = eh * scale;
		imgLeft = (cw - imgW) / 2;
		imgTop = (ch - imgH) / 2;

		// Fabric rotates around the origin point (top-left).
		// We need to offset so the rotated image is centered correctly.
		let left = imgLeft, top = imgTop;
		if (rotation === 90) { left = imgLeft + imgW; }
		else if (rotation === 180) { left = imgLeft + imgW; top = imgTop + imgH; }
		else if (rotation === 270) { top = imgTop + imgH; }

		fabricImage.set({
			left, top,
			scaleX: scale, scaleY: scale,
			selectable: false, evented: false,
			originX: 'left', originY: 'top',
			angle: rotation
		});
	}

	function createCropRect(useSaved: boolean = false) {
		if (!fabricCanvas) return;
		const fabric = (window as any).__fabric;
		if (!fabric) return;

		if (cropRect) fabricCanvas.remove(cropRect);

		const aspect = cropAspect();
		let rectW: number, rectH: number, rectL: number, rectT: number;

		if (useSaved && savedEdits?.transform) {
			// Restore saved crop position
			rectW = cropWidth * imgW;
			rectH = cropHeight * imgH;
			rectL = imgLeft + cropX * imgW;
			rectT = imgTop + cropY * imgH;
		} else {
			// Default: largest possible crop, centered
			if (imgW / imgH > aspect) {
				rectH = imgH;
				rectW = rectH * aspect;
			} else {
				rectW = imgW;
				rectH = rectW / aspect;
			}
			rectW = Math.min(rectW, cw);
			rectH = Math.min(rectH, ch);
			rectL = imgLeft + (imgW - rectW) / 2;
			rectT = imgTop + (imgH - rectH) / 2;
		}

		cropRect = new fabric.Rect({
			left: rectL, top: rectT, width: rectW, height: rectH,
			fill: 'rgba(255,255,255,0.05)',
			stroke: '#fff', strokeWidth: 2, strokeDashArray: [8, 4],
			cornerColor: '#4a9eff', cornerStrokeColor: '#fff', cornerSize: 18,
			transparentCorners: false, lockRotation: true,
			originX: 'left', originY: 'top'
		});

		cropRect.setControlsVisibility({ mt: false, mb: false, ml: false, mr: false, mtr: false });

		cropRect.on('scaling', () => {
			const sx = cropRect.scaleX || 1;
			const newW = cropRect.width! * sx;
			const newH = newW / aspect;
			cropRect.set({
				scaleX: 1, scaleY: 1,
				width: Math.max(40, Math.min(newW, cw)),
				height: Math.max(40 / aspect, Math.min(newH, ch))
			});
			constrainCrop();
		});

		cropRect.on('moving', constrainCrop);
		cropRect.on('modified', constrainCrop);

		fabricCanvas.add(cropRect);
		fabricCanvas.setActiveObject(cropRect);
	}

	function constrainCrop() {
		if (!cropRect) return;
		const w = cropRect.width! * (cropRect.scaleX || 1);
		const h = cropRect.height! * (cropRect.scaleY || 1);
		cropRect.set({
			left: Math.max(0, Math.min(cropRect.left!, cw - w)),
			top: Math.max(0, Math.min(cropRect.top!, ch - h))
		});
		cropRect.setCoords();
	}

	function toggleOrientation() {
		orientation = orientation === 'landscape' ? 'portrait' : 'landscape';
		createCropRect();
		fabricCanvas?.renderAll();
	}

	function rotateImage() {
		rotation = (rotation + 90) % 360;
		if (!fabricImage || !fabricCanvas) return;

		// Resize canvas to fit rotated image
		const iw = fabricImage.width || 1;
		const ih = fabricImage.height || 1;
		const isSwapped = rotation === 90 || rotation === 270;
		const ew = isSwapped ? ih : iw;
		const eh = isSwapped ? iw : ih;
		const imageAspect = ew / eh;

		// Keep canvas width fixed, adjust height to show full image
		ch = Math.round(cw / imageAspect);
		ch = Math.max(ch, 200); // minimum height
		ch = Math.min(ch, 600); // maximum height
		fabricCanvas.setDimensions({ width: cw, height: ch });

		layoutImage();
		fabricImage.setCoords();
		createCropRect();
		fabricCanvas.renderAll();
	}

	function maximizeCrop() {
		if (!cropRect || !fabricCanvas) return;
		const aspect = cropAspect();
		let rectW: number, rectH: number;
		if (cw / ch > aspect) { rectH = ch; rectW = rectH * aspect; }
		else { rectW = cw; rectH = rectW / aspect; }
		cropRect.set({
			left: (cw - rectW) / 2, top: (ch - rectH) / 2,
			width: rectW, height: rectH, scaleX: 1, scaleY: 1
		});
		cropRect.setCoords();
		fabricCanvas.renderAll();
	}

	function getCropValues(): CropTransform {
		if (!cropRect || !fabricImage || imgW <= 0 || imgH <= 0) {
			console.warn('getCropValues: canvas not ready, using state values');
			return { crop_x: cropX, crop_y: cropY, crop_width: cropWidth, crop_height: cropHeight, rotation };
		}
		const rectL = cropRect.left!;
		const rectT = cropRect.top!;
		const rectW = cropRect.width! * (cropRect.scaleX || 1);
		const rectH = cropRect.height! * (cropRect.scaleY || 1);
		return {
			crop_x: Math.max(0, Math.min(1, (rectL - imgLeft) / imgW)),
			crop_y: Math.max(0, Math.min(1, (rectT - imgTop) / imgH)),
			crop_width: Math.max(0.01, Math.min(1, rectW / imgW)),
			crop_height: Math.max(0.01, Math.min(1, rectH / imgH)),
			rotation
		};
	}

	async function switchToAdjust() {
		activeTab = 'adjust';
		renderLoading = true;

		// Save current crop so the render endpoint uses it
		const crop = getCropValues();
		try {
			await saveTransform(photoId, crop);
		} catch { /* ignore */ }

		// Load render preview (cache-bust with timestamp)
		renderSrc = renderPreviewUrl(photoId) + '?t=' + Date.now();
		renderLoading = false;
	}

	async function handleSave() {
		saving = true;

		const crop = getCropValues();

		const edits: EditState = {
			crop_x: crop.crop_x,
			crop_y: crop.crop_y,
			crop_width: crop.crop_width,
			crop_height: crop.crop_height,
			rotation: crop.rotation,
			brightness: brightness,
			contrast: contrast,
			saturation: saturation,
			overlay_overrides: [],
			text_overrides: [],
			copies: copies
		};

		try {
			await saveEdits(photoId, edits);
			onSave();
		} catch (e) {
			console.error('Save failed:', e);
		}
		saving = false;
	}

	onDestroy(() => {
		fabricCanvas?.dispose();
		delete (window as any).__fabric;
	});
</script>

<div class="editor" bind:this={containerEl}>
	{#if loading}
		<div class="loading">
			<div class="spinner"></div>
			<p>Loading editor...</p>
		</div>
	{/if}

	<!-- Canvas — always in DOM, hidden when not on crop tab or loading -->
	<div class="canvas-area" class:hidden={loading || activeTab !== 'crop'}>
		<canvas bind:this={canvasEl}></canvas>
		<div class="controls">
			<button class="ghost ctrl-btn" onclick={toggleOrientation}>
				{orientation === 'landscape' ? 'Landscape' : 'Portrait'}
			</button>
			<button class="ghost ctrl-btn" onclick={rotateImage}>Rotate {rotation}°</button>
			<button class="ghost ctrl-btn" onclick={maximizeCrop}>Max</button>
		</div>
	</div>

	<!-- Render preview — shown on adjust tab -->
	{#if activeTab === 'adjust'}
		<div class="render-preview">
			{#if renderLoading}
				<div class="preview-loading">
					<div class="spinner"></div>
				</div>
			{:else if renderSrc}
				<img
					src={renderSrc}
					alt="Preview"
					style="filter: {cssFilter()}"
				/>
			{/if}
		</div>
	{/if}

	{#if !loading}
		<!-- Tabs -->
		<div class="tabs">
			<button class="tab" class:active={activeTab === 'crop'} onclick={() => activeTab = 'crop'}>Crop</button>
			<button class="tab" class:active={activeTab === 'adjust'} onclick={switchToAdjust}>Adjust</button>
		</div>

		<!-- Adjust tab -->
		{#if activeTab === 'adjust'}
			<div class="adjust-panel">
				<label class="slider-group">
					<div class="slider-header">
						<span>Brightness: {brightness !== null ? brightness.toFixed(2) : `template (${projectBrightness.toFixed(2)})`}</span>
						{#if brightness !== null}
							<button class="reset-btn" onclick={() => brightness = null}>Reset</button>
						{/if}
					</div>
					<input type="range" min="-1" max="1" step="0.05"
						value={brightness ?? projectBrightness}
						oninput={(e) => brightness = Number((e.target as HTMLInputElement).value)}
					/>
				</label>

				<label class="slider-group">
					<div class="slider-header">
						<span>Contrast: {contrast !== null ? contrast.toFixed(2) : `template (${projectContrast.toFixed(2)})`}</span>
						{#if contrast !== null}
							<button class="reset-btn" onclick={() => contrast = null}>Reset</button>
						{/if}
					</div>
					<input type="range" min="-1" max="1" step="0.05"
						value={contrast ?? projectContrast}
						oninput={(e) => contrast = Number((e.target as HTMLInputElement).value)}
					/>
				</label>

				<label class="slider-group">
					<div class="slider-header">
						<span>Saturation: {saturation !== null ? saturation.toFixed(2) : `template (${projectSaturation.toFixed(2)})`}</span>
						{#if saturation !== null}
							<button class="reset-btn" onclick={() => saturation = null}>Reset</button>
						{/if}
					</div>
					<input type="range" min="-1" max="1" step="0.05"
						value={saturation ?? projectSaturation}
						oninput={(e) => saturation = Number((e.target as HTMLInputElement).value)}
					/>
				</label>
			</div>
		{/if}

		<!-- Copies + Save -->
		<div class="save-bar">
			<label class="copies-field">
				<span>Copies</span>
				<input type="number" min="1" max="99" bind:value={copies} />
			</label>
			<button class="primary save-btn" onclick={handleSave} disabled={saving || !canvasReady}>
				{saving ? 'Saving...' : !canvasReady ? 'Loading...' : 'Save'}
			</button>
		</div>
	{/if}
</div>

<style>
	.loading {
		text-align: center;
		padding: 48px 0;
	}

	.loading p { margin-top: 12px; color: var(--text-muted); }

	.spinner {
		width: 40px; height: 40px;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
		margin: 0 auto;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.editor {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		width: 100%;
	}

	.canvas-area {
		display: flex;
		flex-direction: column;
		align-items: center;
	}

	.canvas-area.hidden {
		height: 0;
		overflow: hidden;
	}

	.render-preview {
		width: 100%;
		background: #000;
		border-radius: var(--radius-sm);
		overflow: hidden;
		min-height: 200px;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.render-preview img {
		width: 100%;
		max-height: 360px;
		object-fit: contain;
		transition: filter 0.1s;
	}

	.preview-loading {
		padding: 48px;
	}

	canvas {
		border-radius: var(--radius-sm);
		touch-action: none;
		max-width: 100%;
	}

	.controls {
		display: flex;
		gap: 8px;
		justify-content: center;
	}

	.ctrl-btn {
		padding: 6px 14px;
		font-size: 0.8rem;
		min-height: auto;
	}

	.tabs {
		display: flex;
		border-bottom: 1px solid var(--border);
		width: 100%;
	}

	.tab {
		flex: 1;
		padding: 10px;
		background: none;
		color: var(--text-muted);
		font-weight: 500;
		font-size: 0.875rem;
		border-bottom: 2px solid transparent;
		border-radius: 0;
		min-height: 44px;
	}

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.adjust-panel {
		display: flex;
		flex-direction: column;
		gap: 16px;
		padding: 8px 0;
		width: 100%;
	}

	.slider-group {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.slider-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.slider-header span {
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.reset-btn {
		padding: 2px 8px;
		font-size: 0.7rem;
		min-height: auto;
		min-width: auto;
		background: transparent;
		color: var(--accent);
		border: 1px solid var(--accent);
		border-radius: 4px;
	}

	.slider-group input[type="range"] {
		width: 100%;
		min-height: auto;
		padding: 0;
		border: none;
		background: transparent;
	}

	.save-bar {
		display: flex;
		align-items: flex-end;
		gap: 12px;
		padding-top: 8px;
		border-top: 1px solid var(--border);
		width: 100%;
	}

	.copies-field {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.copies-field span {
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	.copies-field input {
		width: 60px;
		padding: 8px;
		text-align: center;
		font-size: 0.9rem;
		min-height: auto;
	}

	.save-btn {
		flex: 1;
		padding: 12px 24px;
		font-size: 1rem;
	}
</style>
