<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import type { Overlay, TextOverlay } from '$lib/api';

	interface Props {
		overlays: Overlay[];
		textOverlays: TextOverlay[];
		/**
		 * What to draw for a given text overlay. Date-sourced overlays have no
		 * literal text, so the caller supplies a sample string rendered by the
		 * backend formatter. Falls back to the overlay's own text.
		 */
		textPreview?: (t: TextOverlay) => string;
		lockAspect?: Record<number, boolean>;
		portrait?: boolean;
		onOverlayUpdate: (id: number, data: { x: number; y: number; width: number; height: number; opacity: number }) => void;
		onTextUpdate: (id: number, data: { x: number; y: number; font_size?: number }) => void;
	}

	let { overlays, textOverlays, textPreview, lockAspect = {}, portrait = false, onOverlayUpdate, onTextUpdate }: Props = $props();

	let canvasEl: HTMLCanvasElement;
	let containerEl: HTMLDivElement;
	let fabricCanvas: any;
	let fabricMod: any;
	let cw = 0;
	let ch = 0;
	let initialized = false;

	// Fabric objects currently on the canvas, keyed by "o:<id>" / "t:<id>".
	// Editing a property used to recreate this whole component, which tore down
	// the canvas, re-fetched every overlay image, and made the page jump. Now
	// the objects are updated in place and only genuine additions/removals
	// touch the canvas.
	const objects = new Map<string, any>();
	// Keys with an image load in flight, so overlapping syncs can't add twice.
	const pending = new Set<string>();
	let syncedAspect = 0;

	const ASPECT = $derived(portrait ? 2 / 3 : 3 / 2);

	const overlayKey = (id: number) => `o:${id}`;
	const textKey = (id: number) => `t:${id}`;

	function clamp(v: number): number {
		return Math.max(0, Math.min(1, v));
	}

	// Resolve a font file path to something the canvas can render. Only used for
	// the preview — the print uses the real font file server-side.
	function cssFontFor(fontFamily: string | undefined): string {
		if (!fontFamily) return 'sans-serif';
		const parts = fontFamily.split('/');
		let fname = parts[parts.length - 1].replace(/\.(ttf|otf|ttc)$/i, '');
		for (const suffix of [' Bold Italic', ' Bold', ' Italic', ' Regular', ' Light', ' Medium', ' Thin', ' Black']) {
			if (fname.endsWith(suffix)) fname = fname.slice(0, -suffix.length);
		}
		return fname;
	}

	// originX mirrors the stored anchor, so `left` is whichever edge is pinned.
	// Fabric reports that same edge back on drag, keeping the round trip
	// consistent without any conversion on either side.
	function originXFor(align: string | undefined): 'left' | 'center' | 'right' {
		return align === 'right' ? 'right' : align === 'center' ? 'center' : 'left';
	}

	function reportOverlay(id: number, obj: any) {
		const pixW = (obj.width || 1) * (obj.scaleX || 1);
		const pixH = (obj.height || 1) * (obj.scaleY || 1);
		onOverlayUpdate(id, {
			x: clamp((obj.left || 0) / cw),
			y: clamp((obj.top || 0) / ch),
			width: clamp(pixW / cw),
			height: clamp(pixH / ch),
			opacity: obj.opacity ?? 1
		});
	}

	/**
	 * A plain, non-reactive copy of everything the canvas needs to draw.
	 *
	 * The sync is async (overlay images load over HTTP), and Svelte stops
	 * tracking reads after the first await. So every reactive field is read here
	 * instead, synchronously inside the effect — that read is what registers the
	 * dependency. Reading only the arrays tracked their identity but none of the
	 * fields on them, which is why editing size or opacity left the canvas
	 * stale while dragging still worked.
	 */
	type OverlaySnap = {
		id: number; x: number; y: number;
		width: number; height: number; opacity: number; locked: boolean;
	};
	type TextSnap = {
		id: number; x: number; y: number; fontSize: number; // as stored, unscaled
		color: string; opacity: number; cssFont: string;
		originX: 'left' | 'center' | 'right'; content: string;
	};
	type Snapshot = { aspect: number; overlays: OverlaySnap[]; texts: TextSnap[] };

	// Guards against an older in-flight sync finishing after a newer one.
	let syncSeq = 0;

	async function initCanvas(snap: Snapshot) {
		if (initialized) return;
		initialized = true;

		fabricMod = await import('fabric');

		// Use a fixed width if container isn't laid out yet
		const containerWidth = containerEl?.clientWidth;
		cw = Math.min(containerWidth > 10 ? containerWidth : 480, 480);
		ch = Math.round(cw / snap.aspect);
		syncedAspect = snap.aspect;

		fabricCanvas = new fabricMod.Canvas(canvasEl, {
			width: cw,
			height: ch,
			backgroundColor: '#1a1a1a',
			selection: true,
			uniformScaling: false
		});

		await syncObjects(snap);
	}

	/**
	 * Bring the canvas in line with a snapshot: update what exists, create
	 * what's new, drop what's gone. Selection is preserved so tweaking a value
	 * doesn't deselect the thing being edited.
	 */
	async function syncObjects(snap: Snapshot) {
		if (!fabricCanvas || !fabricMod) return;
		const seq = ++syncSeq;

		// Orientation switch changes the canvas shape. Positions are stored
		// normalized, so re-applying them below repositions everything.
		if (snap.aspect !== syncedAspect) {
			syncedAspect = snap.aspect;
			ch = Math.round(cw / snap.aspect);
			fabricCanvas.setDimensions({ width: cw, height: ch });
		}

		const activeKey = fabricCanvas.getActiveObject()?._key;

		const wanted = new Set<string>();
		for (const ov of snap.overlays) wanted.add(overlayKey(ov.id));
		for (const t of snap.texts) wanted.add(textKey(t.id));

		// Remove objects whose source is gone (deleted, or filtered out by an
		// orientation switch).
		for (const [key, obj] of [...objects]) {
			if (!wanted.has(key)) {
				fabricCanvas.remove(obj);
				objects.delete(key);
			}
		}

		// Text is synchronous, so apply it before awaiting any image load.
		for (const t of snap.texts) upsertText(t);

		for (const ov of snap.overlays) {
			await upsertOverlay(ov);
			// A newer snapshot arrived while an image was loading; it owns the
			// canvas from here.
			if (seq !== syncSeq) return;
		}

		if (activeKey) {
			const restore = objects.get(activeKey);
			if (restore) fabricCanvas.setActiveObject(restore);
		}

		fabricCanvas.requestRenderAll();
	}

	async function upsertOverlay(ov: OverlaySnap) {
		const key = overlayKey(ov.id);

		const existing = objects.get(key);
		if (existing) {
			const scaleX = (ov.width * cw) / (existing.width || 1);
			const scaleY = (ov.height * ch) / (existing.height || 1);
			existing.set({
				left: ov.x * cw,
				top: ov.y * ch,
				scaleX,
				scaleY,
				opacity: ov.opacity
			});
			// Handlers read these off the object rather than closing over values
			// captured at creation, which would go stale as props change.
			existing._lockAspect = ov.locked;
			existing._scaleRatio = scaleX > 0 ? scaleY / scaleX : 1;
			existing.setCoords();
			return;
		}

		if (pending.has(key)) return;
		pending.add(key);
		try {
			const img = await fabricMod.FabricImage.fromURL(
				`/api/admin/overlays/${ov.id}`,
				{ crossOrigin: 'anonymous' }
			);

			const initScaleX = (ov.width * cw) / (img.width || 1);
			const initScaleY = (ov.height * ch) / (img.height || 1);

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

			img._oid = ov.id;
			img._key = key;
			img._lockAspect = ov.locked;
			img._scaleRatio = initScaleX > 0 ? initScaleY / initScaleX : 1;

			// Anchor edges captured when scaling starts, so uniform scaling can
			// hold the opposite corner still.
			let anchorBottom = 0;
			let anchorRight = 0;
			let anchorCorner = '';

			img.on('mousedown', () => {
				anchorCorner = img.__corner || '';
				anchorBottom = (img.top || 0) + (img.height || 1) * (img.scaleY || 1);
				anchorRight = (img.left || 0) + (img.width || 1) * (img.scaleX || 1);
			});

			img.on('scaling', () => {
				if (!img._lockAspect) return;

				const newScaleY = (img.scaleX || 1) * img._scaleRatio;
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

			img.on('modified', () => reportOverlay(ov.id, img));

			objects.set(key, img);
			fabricCanvas.add(img);
		} catch (e) {
			console.warn('Overlay load failed:', ov.id, e);
		} finally {
			pending.delete(key);
		}
	}

	function upsertText(t: TextSnap) {
		const key = textKey(t.id);
		const props = {
			text: t.content,
			left: t.x * cw,
			top: t.y * ch,
			// Scaled here, not in the snapshot, so it never depends on cw.
			fontSize: t.fontSize * (cw / 600),
			fill: t.color,
			opacity: t.opacity,
			fontFamily: t.cssFont,
			originX: t.originX
		};

		const existing = objects.get(key);
		if (existing) {
			existing.set(props);
			existing.setCoords();
			return;
		}

		const ft = new fabricMod.FabricText(t.content, {
			...props,
			cornerColor: '#4a9eff',
			cornerStrokeColor: '#fff',
			cornerSize: 14,
			transparentCorners: false,
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

		ft._tid = t.id;
		ft._key = key;

		ft.on('modified', () => {
			onTextUpdate(t.id, {
				x: clamp((ft.left || 0) / cw),
				y: clamp((ft.top || 0) / ch)
			});
		});

		objects.set(key, ft);
		fabricCanvas.add(ft);
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
		fabricCanvas.requestRenderAll();

		if (obj._oid) {
			reportOverlay(obj._oid, obj);
		} else if (obj._tid) {
			onTextUpdate(obj._tid, {
				x: clamp((obj.left || 0) / cw),
				y: clamp((obj.top || 0) / ch)
			});
		}
	}

	/**
	 * Every reactive read the canvas depends on happens here, synchronously, so
	 * Svelte registers all of them before the async sync begins.
	 */
	function takeSnapshot(): Snapshot {
		return {
			aspect: ASPECT,
			overlays: overlays.map(o => ({
				id: o.id,
				x: o.x,
				y: o.y,
				width: o.width,
				height: o.height,
				opacity: o.opacity,
				locked: lockAspect[o.id] !== false
			})),
			// textPreview reads the overlay's source and date format, so calling
			// it here keeps those tracked too.
			texts: textOverlays.map(t => ({
				id: t.id,
				x: t.x,
				y: t.y,
				fontSize: t.font_size,
				color: t.color,
				opacity: t.opacity,
				cssFont: cssFontFor(t.font_family),
				originX: originXFor(t.text_align),
				content: textPreview?.(t) || t.text || ' '
			}))
		};
	}

	onMount(() => {
		// Small delay to ensure the DOM is fully laid out (needed when
		// appearing inside a tab that was just switched to)
		const timer = setTimeout(() => initCanvas(takeSnapshot()), 50);
		return () => clearTimeout(timer);
	});

	$effect(() => {
		const snap = takeSnapshot();
		if (initialized && fabricCanvas) void syncObjects(snap);
	});

	onDestroy(() => {
		fabricCanvas?.dispose();
		objects.clear();
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
