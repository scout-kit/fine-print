<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		imageUrl: string;
		onSave: (transform: { crop_x: number; crop_y: number; crop_width: number; crop_height: number; rotation: number }) => void;
	}

	let { imageUrl, onSave }: Props = $props();

	let canvasEl: HTMLCanvasElement;
	let containerEl: HTMLDivElement;
	let fabricCanvas: any;
	let cropRect: any;
	let fabricImage: any;

	// Image position/size on canvas
	let imgLeft = 0, imgTop = 0, imgW = 0, imgH = 0;

	// Orientation: landscape (3:2) or portrait (2:3)
	let orientation = $state<'landscape' | 'portrait'>('landscape');
	let rotation = $state(0); // 0, 90, 180, 270

	let cw = 0, ch = 0;

	function cropAspect(): number {
		return orientation === 'landscape' ? 3 / 2 : 2 / 3;
	}

	onMount(async () => {
		const fabric = await import('fabric');
		(window as any).__fabricModule = fabric;

		await new Promise(r => setTimeout(r, 50));

		cw = Math.min(containerEl.clientWidth || 480, 480);
		ch = Math.round(cw * 0.75);

		fabricCanvas = new fabric.Canvas(canvasEl, {
			width: cw, height: ch,
			backgroundColor: '#000',
			selection: false
		});

		const img = await fabric.FabricImage.fromURL(imageUrl, { crossOrigin: 'anonymous' });
		fabricImage = img;

		// Auto-detect orientation from image dimensions
		const iw = img.width || 1;
		const ih = img.height || 1;
		orientation = iw >= ih ? 'landscape' : 'portrait';

		layoutImage();
		fabricCanvas.add(fabricImage);
		createCropRect();

		// Double-click to maximize crop
		fabricCanvas.on('mouse:dblclick', () => maximizeCrop());

		fabricCanvas.renderAll();
	});

	function layoutImage() {
		if (!fabricImage || !fabricCanvas) return;

		const iw = fabricImage.width || 1;
		const ih = fabricImage.height || 1;

		// Scale image to cover the entire canvas
		const scale = Math.max(cw / iw, ch / ih);
		imgW = iw * scale;
		imgH = ih * scale;
		imgLeft = (cw - imgW) / 2;
		imgTop = (ch - imgH) / 2;

		fabricImage.set({
			left: imgLeft, top: imgTop,
			scaleX: scale, scaleY: scale,
			selectable: false, evented: false,
			originX: 'left', originY: 'top',
			angle: 0
		});
	}

	function createCropRect() {
		if (!fabricCanvas) return;

		// Remove old crop rect
		if (cropRect) {
			fabricCanvas.remove(cropRect);
		}

		const aspect = cropAspect();

		// Size the crop rect to fit within the image area, as large as possible
		let rectW: number, rectH: number;
		const maxW = Math.min(imgW, cw);
		const maxH = Math.min(imgH, ch);

		if (maxW / maxH > aspect) {
			rectH = maxH * 0.85;
			rectW = rectH * aspect;
		} else {
			rectW = maxW * 0.85;
			rectH = rectW / aspect;
		}

		// Clamp to canvas
		rectW = Math.min(rectW, cw);
		rectH = Math.min(rectH, ch);

		const fabric = (window as any).__fabricModule;
		if (!fabric) return;

		cropRect = new fabric.Rect({
			left: (cw - rectW) / 2,
			top: (ch - rectH) / 2,
			width: rectW, height: rectH,
			fill: 'rgba(255,255,255,0.05)',
			stroke: '#fff', strokeWidth: 2,
			strokeDashArray: [8, 4],
			cornerColor: '#4a9eff',
			cornerStrokeColor: '#fff',
			cornerSize: 18,
			transparentCorners: false,
			lockRotation: true,
			originX: 'left', originY: 'top'
		});

		cropRect.setControlsVisibility({
			mt: false, mb: false, ml: false, mr: false, mtr: false
		});

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

		// Rotate the image on canvas
		fabricImage.set({ angle: rotation });

		// Recalculate image bounds after rotation
		const iw = fabricImage.width || 1;
		const ih = fabricImage.height || 1;

		// After 90/270 rotation, effective dimensions swap
		const isSwapped = rotation === 90 || rotation === 270;
		const ew = isSwapped ? ih : iw;
		const eh = isSwapped ? iw : ih;

		const scale = Math.max(cw / ew, ch / eh);
		imgW = ew * scale;
		imgH = eh * scale;
		imgLeft = (cw - imgW) / 2;
		imgTop = (ch - imgH) / 2;

		// Fabric rotates around the object's origin. With originX/Y='left'/'top',
		// we need to offset so the rotated image is centered.
		let left = imgLeft;
		let top = imgTop;
		if (rotation === 90) {
			left = imgLeft + imgW;
		} else if (rotation === 180) {
			left = imgLeft + imgW;
			top = imgTop + imgH;
		} else if (rotation === 270) {
			top = imgTop + imgH;
		}

		fabricImage.set({
			left, top,
			scaleX: scale,
			scaleY: scale
		});

		fabricImage.setCoords();
		createCropRect();
		fabricCanvas.renderAll();
	}

	function maximizeCrop() {
		if (!cropRect || !fabricCanvas) return;

		const aspect = cropAspect();

		// Fit crop rect as large as possible within the canvas
		let rectW: number, rectH: number;
		if (cw / ch > aspect) {
			rectH = ch;
			rectW = rectH * aspect;
		} else {
			rectW = cw;
			rectH = rectW / aspect;
		}

		cropRect.set({
			left: (cw - rectW) / 2,
			top: (ch - rectH) / 2,
			width: rectW,
			height: rectH,
			scaleX: 1,
			scaleY: 1
		});
		cropRect.setCoords();
		fabricCanvas.renderAll();
	}

	function handleSave() {
		if (!cropRect || !fabricImage) return;

		const rectL = cropRect.left!;
		const rectT = cropRect.top!;
		const rectW = cropRect.width! * (cropRect.scaleX || 1);
		const rectH = cropRect.height! * (cropRect.scaleY || 1);

		// Convert crop rect to image-relative normalized coords
		const cx = Math.max(0, Math.min(1, (rectL - imgLeft) / imgW));
		const cy = Math.max(0, Math.min(1, (rectT - imgTop) / imgH));
		const cWn = Math.max(0.01, Math.min(1 - cx, rectW / imgW));
		const cHn = Math.max(0.01, Math.min(1 - cy, rectH / imgH));

		onSave({
			crop_x: cx,
			crop_y: cy,
			crop_width: cWn,
			crop_height: cHn,
			rotation: rotation
		});
	}

	onDestroy(() => {
		fabricCanvas?.dispose();
		delete (window as any).__fabricModule;
	});
</script>

<div class="crop-editor" bind:this={containerEl}>
	<canvas bind:this={canvasEl}></canvas>

	<div class="controls">
		<button class="ghost ctrl-btn" onclick={toggleOrientation}>
			{orientation === 'landscape' ? 'Landscape' : 'Portrait'} (4x6)
		</button>
		<button class="ghost ctrl-btn" onclick={rotateImage}>
			Rotate {rotation}°
		</button>
		<button class="ghost ctrl-btn" onclick={maximizeCrop}>
			Max
		</button>
	</div>

	<div class="crop-info">
		Drag to move, corners to resize
	</div>

	<button class="primary save-btn" onclick={handleSave}>
		Save & Submit
	</button>
</div>

<style>
	.crop-editor {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 12px;
		width: 100%;
	}

	canvas {
		border-radius: var(--radius-sm);
		touch-action: none;
		max-width: 100%;
	}

	.controls {
		display: flex;
		gap: 8px;
	}

	.ctrl-btn {
		padding: 8px 16px;
		font-size: 0.8rem;
		min-height: auto;
	}

	.crop-info {
		color: var(--text-muted);
		font-size: 0.8rem;
	}

	.save-btn {
		width: 100%;
		max-width: 320px;
		padding: 14px 24px;
		font-size: 1.1rem;
	}
</style>
