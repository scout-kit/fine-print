<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Overlay, TextOverlay } from '$lib/api';

	interface Props {
		overlays: Overlay[];
		textOverlays: TextOverlay[];
		lockAspect?: Record<number, boolean>;
		portrait?: boolean;
		onOverlayUpdate: (id: number, data: { x: number; y: number; width: number; height: number; opacity: number }) => void;
		onTextUpdate: (id: number, data: { x: number; y: number; font_size?: number }) => void;
	}

	let { overlays, textOverlays, lockAspect = {}, portrait = false, onOverlayUpdate, onTextUpdate }: Props = $props();

	let canvasEl: HTMLCanvasElement;
	let containerEl: HTMLDivElement;
	let fabricCanvas: any;
	let cw = 0;
	let ch = 0;
	let initialized = false;

	const ASPECT = $derived(portrait ? 2 / 3 : 3 / 2);
	const PRINT_W = 1800;

	async function initCanvas() {
		if (initialized) return;
		initialized = true;

		const fabric = await import('fabric');

		// Use a fixed width if container isn't laid out yet
		const containerWidth = containerEl?.clientWidth;
		cw = Math.min(containerWidth > 10 ? containerWidth : 480, 480);
		ch = Math.round(cw / ASPECT);

		fabricCanvas = new fabric.Canvas(canvasEl, {
			width: cw,
			height: ch,
			backgroundColor: '#1a1a1a',
			selection: true,
			uniformScaling: false
		});

		// === OVERLAYS ===
		for (const ov of overlays) {
			try {
				const img = await fabric.FabricImage.fromURL(
					`/api/admin/overlays/${ov.id}`,
					{ crossOrigin: 'anonymous' }
				);

				const pw = ov.width * cw;
				const ph = ov.height * ch;
				const initScaleX = pw / (img.width || 1);
				const initScaleY = ph / (img.height || 1);

				img.set({
					left: ov.x * cw,
					top: ov.y * ch,
					scaleX: initScaleX,
					scaleY: initScaleY,
					opacity: ov.opacity,
					cornerColor: '#4a9eff',
					cornerStrokeColor: '#fff',
					cornerSize: 14,
					transparentCorners: false,
					originX: 'left',
					originY: 'top'
				});

				// Corner handles only, no edge or rotation
				img.setControlsVisibility({
					mt: false, mb: false, ml: false, mr: false,
					mtr: false
				});

				(img as any)._oid = ov.id;
				const isLocked = lockAspect[ov.id] !== false;
				(img as any)._lockAspect = isLocked;

				const scaleRatio = (initScaleX > 0 && initScaleY > 0) ? initScaleY / initScaleX : 1;

				// Capture anchor edges when scaling starts
				let anchorBottom = 0;
				let anchorRight = 0;
				let anchorCorner = '';

				img.on('mousedown', () => {
					anchorCorner = (img as any).__corner || '';
					anchorBottom = (img.top || 0) + (img.height || 1) * (img.scaleY || 1);
					anchorRight = (img.left || 0) + (img.width || 1) * (img.scaleX || 1);
				});

				img.on('scaling', () => {
					if (!(img as any)._lockAspect) return;

					const newScaleY = (img.scaleX || initScaleX) * scaleRatio;
					img.set({ scaleY: newScaleY });

					// Fix position to keep the opposite corner anchored
					const newW = (img.width || 1) * (img.scaleX || 1);
					const newH = (img.height || 1) * newScaleY;

					if (anchorCorner === 'tl') {
						img.set({ top: anchorBottom - newH, left: anchorRight - newW });
					} else if (anchorCorner === 'tr') {
						img.set({ top: anchorBottom - newH });
					} else if (anchorCorner === 'bl') {
						img.set({ left: anchorRight - newW });
					}
					// br: top-left is already anchored by default
				});

				img.on('modified', () => {
					const pixW = (img.width || 1) * (img.scaleX || 1);
					const pixH = (img.height || 1) * (img.scaleY || 1);
					onOverlayUpdate(ov.id, {
						x: clamp((img.left || 0) / cw),
						y: clamp((img.top || 0) / ch),
						width: clamp(pixW / cw),
						height: clamp(pixH / ch),
						opacity: img.opacity ?? 1
					});
				});

				fabricCanvas.add(img);
			} catch (e) {
				console.warn('Overlay load failed:', ov.id, e);
			}
		}

		// === TEXT OVERLAYS ===
		for (const t of textOverlays) {
			const scaledSize = t.font_size * (cw / 600);

			// Convert file path to CSS font name for canvas preview
			let cssFont = 'sans-serif';
			if (t.font_family) {
				// Extract filename without path and extension
				const parts = t.font_family.split('/');
				let fname = parts[parts.length - 1];
				fname = fname.replace(/\.(ttf|otf|ttc)$/i, '');
				// Strip style suffixes
				for (const s of [' Bold Italic', ' Bold', ' Italic', ' Regular', ' Light', ' Medium', ' Thin', ' Black']) {
					if (fname.endsWith(s)) fname = fname.slice(0, -s.length);
				}
				cssFont = fname;
			}

			const ft = new fabric.FabricText(t.text, {
				left: t.x * cw,
				top: t.y * ch,
				fontSize: scaledSize,
				fill: t.color,
				opacity: t.opacity,
				fontFamily: cssFont,
				cornerColor: '#4a9eff',
				cornerStrokeColor: '#fff',
				cornerSize: 14,
				transparentCorners: false,
				originX: 'left',
				originY: 'top',
				lockRotation: true,
				lockScalingX: true,
				lockScalingY: true
			});

			ft.setControlsVisibility({
				mt: false, mb: false, ml: false, mr: false,
				tl: false, tr: false, bl: false, br: false,
				mtr: false
			});

			(ft as any)._tid = t.id;

			ft.on('modified', () => {
				onTextUpdate(t.id, {
					x: clamp((ft.left || 0) / cw),
					y: clamp((ft.top || 0) / ch)
				});
			});

			fabricCanvas.add(ft);
		}

		fabricCanvas.renderAll();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (!fabricCanvas) return;
		const obj = fabricCanvas.getActiveObject();
		if (!obj) return;

		const step = e.shiftKey ? 10 : 1;
		let dx = 0, dy = 0;

		switch (e.key) {
			case 'ArrowLeft':  dx = -step; break;
			case 'ArrowRight': dx = step; break;
			case 'ArrowUp':    dy = -step; break;
			case 'ArrowDown':  dy = step; break;
			default: return;
		}

		e.preventDefault();
		obj.set({ left: (obj.left || 0) + dx, top: (obj.top || 0) + dy });
		obj.setCoords();
		fabricCanvas.renderAll();

		const oid = (obj as any)._oid;
		const tid = (obj as any)._tid;

		if (oid) {
			const pixW = (obj.width || 1) * (obj.scaleX || 1);
			const pixH = (obj.height || 1) * (obj.scaleY || 1);
			onOverlayUpdate(oid, {
				x: clamp((obj.left || 0) / cw),
				y: clamp((obj.top || 0) / ch),
				width: clamp(pixW / cw),
				height: clamp(pixH / ch),
				opacity: obj.opacity ?? 1
			});
		} else if (tid) {
			onTextUpdate(tid, {
				x: clamp((obj.left || 0) / cw),
				y: clamp((obj.top || 0) / ch)
			});
		}
	}

	function clamp(v: number): number {
		return Math.max(0, Math.min(1, v));
	}

	onMount(() => {
		// Small delay to ensure the DOM is fully laid out (needed when
		// appearing inside a tab that was just switched to)
		const timer = setTimeout(() => initCanvas(), 50);
		return () => clearTimeout(timer);
	});

	onDestroy(() => {
		fabricCanvas?.dispose();
	});
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="overlay-editor" bind:this={containerEl} onkeydown={handleKeydown} tabindex="0">
	<p class="editor-label">Template Preview (4x6)</p>
	<canvas bind:this={canvasEl}></canvas>
	<p class="editor-hint">Drag to move, corners to resize. Arrow keys for fine adjustment (Shift = 10px).</p>
</div>

<style>
	.overlay-editor {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.editor-label {
		font-size: 0.8rem;
		color: var(--text-muted);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	canvas {
		border-radius: var(--radius-sm);
		border: 1px solid var(--border);
		touch-action: none;
		max-width: 100%;
	}

	.editor-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
	}
</style>
