<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import OverlayEditor from '$lib/OverlayEditor.svelte';
	import NumberStepper from '$lib/NumberStepper.svelte';
	import {
		getProject, uploadOverlay, updateOverlayPosition, deleteOverlay,
		createTextOverlay, updateTextOverlay, deleteTextOverlay,
		copyTemplateOrientation, listAvailableFonts, listDateFormats,
		ORIENTATION_LANDSCAPE, ORIENTATION_PORTRAIT,
		TEXT_SOURCE_STATIC, TEXT_SOURCE_PHOTO_DATE, TEXT_SOURCE_PHOTO_DATETIME,
		TEXT_ALIGN_LEFT, TEXT_ALIGN_CENTER, TEXT_ALIGN_RIGHT,
		type ProjectResponse, type Overlay, type TextOverlay, type SystemFont,
		type TextSourceOption, type TextOverlaySource, type DateFormatOption,
		type TextAlign
	} from '$lib/api';

	const projectId = $derived(Number(page.params.id));
	let data: ProjectResponse | null = $state(null);
	let orientation = $state(ORIENTATION_LANDSCAPE);

	let newText = $state('');
	let newFontSize = $state(25);
	let newTextColor = $state('#FFFFFF');
	let newFontFamily = $state('');
	let newSource: TextOverlaySource = $state(TEXT_SOURCE_STATIC);
	let newDateFormat = $state('');
	let newAlign: TextAlign = $state(TEXT_ALIGN_LEFT);
	let fonts: SystemFont[] = $state([]);

	// Date presets come from the backend so the examples shown here are
	// rendered by the same formatter that renders the print.
	let sourceOptions: TextSourceOption[] = $state([]);

	function formatsFor(source: TextOverlaySource): DateFormatOption[] {
		return sourceOptions.find(s => s.key === source)?.formats ?? [];
	}

	function defaultFormatFor(source: TextOverlaySource): string {
		return formatsFor(source).find(f => f.default)?.key ?? '';
	}

	function sourceLabel(source: TextOverlaySource): string {
		return sourceOptions.find(s => s.key === source)?.label ?? source;
	}

	function isDateSource(source: TextOverlaySource | undefined): boolean {
		return source === TEXT_SOURCE_PHOTO_DATE || source === TEXT_SOURCE_PHOTO_DATETIME;
	}

	// x is the anchored edge, so this decides which way the text grows when its
	// content gets longer.
	const ALIGN_OPTIONS: { value: TextAlign; label: string; hint: string }[] = [
		{ value: TEXT_ALIGN_LEFT,   label: 'Left',   hint: 'x is the left edge — text grows right' },
		{ value: TEXT_ALIGN_CENTER, label: 'Center', hint: 'x is the midpoint — text grows evenly both ways' },
		{ value: TEXT_ALIGN_RIGHT,  label: 'Right',  hint: 'x is the right edge — text grows left' }
	];

	function alignHint(align: TextAlign | undefined): string {
		return ALIGN_OPTIONS.find(o => o.value === (align || TEXT_ALIGN_LEFT))?.hint ?? '';
	}

	/**
	 * Example string for a preset key, used for the canvas preview and the
	 * overlay list. Falls back to the source default when no format is stored.
	 */
	function exampleFor(source: TextOverlaySource, formatKey: string): string {
		const formats = formatsFor(source);
		return (formats.find(f => f.key === formatKey) ?? formats.find(f => f.default))?.example ?? '';
	}

	// The canvas has no literal text to draw for a date overlay, so give it the
	// example rendering instead.
	function textPreview(t: TextOverlay): string {
		if (!isDateSource(t.source)) return t.text;
		return exampleFor(t.source, t.date_format) || t.text;
	}

	// Switching source resets the format to that source's default, so a
	// date-only source can never keep a time-bearing preset (the API rejects it).
	function onNewSourceChange(source: TextOverlaySource) {
		newSource = source;
		newDateFormat = defaultFormatFor(source);
	}

	function onEditSourceChange(id: number, source: TextOverlaySource) {
		updateTextProp(id, 'source', source);
		updateTextProp(id, 'date_format', defaultFormatFor(source));
	}

	let editingOverlayId = $state<number | null>(null);
	let editingTextId = $state<number | null>(null);
	let lockAspect = $state<Record<number, boolean>>({});

	// Filter overlays/text by current orientation
	let filteredOverlays = $derived(
		(data?.overlays || []).filter(o => (o.orientation_id || 1) === orientation)
	);
	let filteredTextOverlays = $derived(
		(data?.text_overlays || []).filter(t => (t.orientation_id || 1) === orientation)
	);

	async function load() {
		try {
			data = await getProject(projectId);
		} catch { data = null; }
	}

	onMount(async () => {
		load();
		try { fonts = await listAvailableFonts(); } catch { /* ignore */ }
		try { sourceOptions = (await listDateFormats()).sources; } catch { /* ignore */ }
	});

	// ---------------------------------------------------------------------
	// Autosave + undo
	//
	// Every edit persists on its own; there is no Save button. Previously the
	// canvas saved on drag while the side panel waited for Save, which meant a
	// later drag quietly persisted panel edits that were never committed. One
	// rule now: what you see is what's stored.
	//
	// Undo is what makes that safe. A burst of edits — holding a stepper, or
	// dragging — coalesces into a single undo entry, captured before the burst
	// starts and pushed when the burst is flushed to the server.
	// ---------------------------------------------------------------------

	const SAVE_DEBOUNCE_MS = 400;
	const UNDO_LIMIT = 50;

	type OverlayFields = Pick<Overlay, 'x' | 'y' | 'width' | 'height' | 'opacity'>;
	type TextFields = Pick<TextOverlay,
		'text' | 'font_family' | 'font_size' | 'color' | 'x' | 'y' | 'opacity' |
		'source' | 'date_format' | 'text_align'>;
	type EditSnapshot = {
		overlays: Map<number, OverlayFields>;
		texts: Map<number, TextFields>;
	};

	let undoStack: EditSnapshot[] = $state([]);
	let redoStack: EditSnapshot[] = $state([]);
	let saveState: 'idle' | 'pending' | 'saving' | 'error' = $state('idle');

	// State as it was before the in-progress burst of edits.
	let burstBaseline: EditSnapshot | null = null;
	let dirtyOverlays = new Set<number>();
	let dirtyTexts = new Set<number>();
	let saveTimer: ReturnType<typeof setTimeout> | null = null;

	const canUndo = $derived(undoStack.length > 0);
	const canRedo = $derived(redoStack.length > 0);

	function overlayFields(o: Overlay): OverlayFields {
		return { x: o.x, y: o.y, width: o.width, height: o.height, opacity: o.opacity };
	}

	function textFields(t: TextOverlay): TextFields {
		return {
			text: t.text, font_family: t.font_family, font_size: t.font_size,
			color: t.color, x: t.x, y: t.y, opacity: t.opacity,
			source: t.source, date_format: t.date_format, text_align: t.text_align
		};
	}

	function takeEditSnapshot(): EditSnapshot {
		return {
			overlays: new Map((data?.overlays || []).map(o => [o.id, overlayFields(o)])),
			texts: new Map((data?.text_overlays || []).map(t => [t.id, textFields(t)]))
		};
	}

	/**
	 * Record that something changed and schedule a save. The first call of a
	 * burst captures the pre-edit state so the whole burst undoes as one step.
	 */
	function markDirty(kind: 'overlay' | 'text', id: number) {
		if (!burstBaseline) burstBaseline = takeEditSnapshot();
		if (kind === 'overlay') dirtyOverlays.add(id); else dirtyTexts.add(id);
		saveState = 'pending';
		if (saveTimer) clearTimeout(saveTimer);
		saveTimer = setTimeout(() => void flushSaves(), SAVE_DEBOUNCE_MS);
	}

	async function flushSaves() {
		saveTimer = null;
		const overlayIds = [...dirtyOverlays];
		const textIds = [...dirtyTexts];
		dirtyOverlays = new Set();
		dirtyTexts = new Set();

		if (burstBaseline) {
			undoStack = [...undoStack, burstBaseline].slice(-UNDO_LIMIT);
			// A fresh edit invalidates any redo branch.
			redoStack = [];
			burstBaseline = null;
		}

		if (!overlayIds.length && !textIds.length) { saveState = 'idle'; return; }

		saveState = 'saving';
		try {
			for (const id of overlayIds) {
				const o = getOverlay(id);
				if (o) await updateOverlayPosition(id, overlayFields(o));
			}
			for (const id of textIds) {
				const t = getText(id);
				if (t) await updateTextOverlay(id, textFields(t));
			}
			saveState = 'idle';
		} catch (e) {
			console.error('Autosave failed:', e);
			saveState = 'error';
		}
	}

	/**
	 * Write a snapshot to the server, touching only the records that actually
	 * differ from `from`. Undoing a one-overlay tweak shouldn't PUT every
	 * overlay in the template.
	 */
	async function persistSnapshot(snap: EditSnapshot, from: EditSnapshot) {
		const changed = <T extends object>(a: T | undefined, b: T | undefined) =>
			JSON.stringify(a ?? null) !== JSON.stringify(b ?? null);

		saveState = 'saving';
		try {
			for (const [id, f] of snap.overlays) {
				if (getOverlay(id) && changed(f, from.overlays.get(id))) {
					await updateOverlayPosition(id, f);
				}
			}
			for (const [id, f] of snap.texts) {
				if (getText(id) && changed(f, from.texts.get(id))) {
					await updateTextOverlay(id, f);
				}
			}
			saveState = 'idle';
		} catch (e) {
			console.error('Undo save failed:', e);
			saveState = 'error';
		}
	}

	/** Apply a snapshot to local state. Ids that no longer exist are skipped. */
	function applySnapshot(snap: EditSnapshot) {
		for (const [id, f] of snap.overlays) {
			const o = getOverlay(id);
			if (o) Object.assign(o, f);
		}
		for (const [id, f] of snap.texts) {
			const t = getText(id);
			if (t) Object.assign(t, f);
		}
	}

	async function undo() {
		// Land any in-flight burst first, so its undo entry exists.
		if (saveTimer) { clearTimeout(saveTimer); await flushSaves(); }
		const prev = undoStack.at(-1);
		if (!prev) return;
		const current = takeEditSnapshot();
		undoStack = undoStack.slice(0, -1);
		redoStack = [...redoStack, current];
		applySnapshot(prev);
		await persistSnapshot(prev, current);
	}

	async function redo() {
		if (saveTimer) { clearTimeout(saveTimer); await flushSaves(); }
		const next = redoStack.at(-1);
		if (!next) return;
		const current = takeEditSnapshot();
		redoStack = redoStack.slice(0, -1);
		undoStack = [...undoStack, current];
		applySnapshot(next);
		await persistSnapshot(next, current);
	}

	/**
	 * Adding or deleting an overlay changes which ids exist, so old snapshots
	 * can no longer be applied faithfully. Those actions aren't undoable.
	 */
	function clearHistory() {
		undoStack = [];
		redoStack = [];
		burstBaseline = null;
		dirtyOverlays = new Set();
		dirtyTexts = new Set();
		if (saveTimer) { clearTimeout(saveTimer); saveTimer = null; }
		saveState = 'idle';
	}

	function handleWindowKeydown(e: KeyboardEvent) {
		if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== 'z') return;
		// Leave text fields to their own native undo.
		const el = e.target as HTMLElement | null;
		if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)) return;
		e.preventDefault();
		if (e.shiftKey) void redo(); else void undo();
	}

	// Persist anything still pending if the page is being closed.
	function flushBeforeUnload() {
		if (saveTimer) { clearTimeout(saveTimer); void flushSaves(); }
	}

	onDestroy(() => { if (saveTimer) clearTimeout(saveTimer); });

	// ---------------------------------------------------------------------
	// Overlay handlers
	// ---------------------------------------------------------------------
	async function handleOverlayUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.[0]) return;
		await uploadOverlay(projectId, input.files[0], orientation);
		input.value = '';
		clearHistory();
		load();
	}

	async function handleCopyOrientation() {
		const from = orientation;
		const to = orientation === ORIENTATION_LANDSCAPE ? ORIENTATION_PORTRAIT : ORIENTATION_LANDSCAPE;
		await copyTemplateOrientation(projectId, from, to);
		clearHistory();
		load();
	}

	function handleOverlayDrag(id: number, posData: { x: number; y: number; width: number; height: number; opacity: number }) {
		const o = getOverlay(id);
		if (!o) return;
		o.x = posData.x; o.y = posData.y;
		o.width = posData.width; o.height = posData.height;
		o.opacity = posData.opacity;
		markDirty('overlay', id);
	}

	async function handleOverlayDelete(id: number) {
		editingOverlayId = null;
		await deleteOverlay(id);
		clearHistory();
		load();
	}

	function startEditOverlay(id: number) { editingOverlayId = editingOverlayId === id ? null : id; editingTextId = null; }
	function getOverlay(id: number): Overlay | undefined { return data?.overlays?.find(x => x.id === id); }
	function isLocked(id: number): boolean { return lockAspect[id] !== false; }
	function toggleLock(id: number) { lockAspect = { ...lockAspect, [id]: !isLocked(id) }; }

	function updateOverlayProp(id: number, prop: 'x' | 'y' | 'width' | 'height' | 'opacity', value: number) {
		const o = getOverlay(id);
		if (!o) return;
		if (prop === 'width' && isLocked(id) && o.width > 0) { o.height = Math.max(0.01, Math.min(1, o.height * (value / o.width))); }
		else if (prop === 'height' && isLocked(id) && o.height > 0) { o.width = Math.max(0.01, Math.min(1, o.width * (value / o.height))); }
		(o as any)[prop] = Math.max(0, Math.min(1, value));
		markDirty('overlay', id);
	}

	function snapOverlay(id: number, corner: 'tl' | 'tr' | 'bl' | 'br') {
		const o = getOverlay(id);
		if (!o) return;
		const snap: Record<string, { x: number; y: number }> = {
			tl: { x: 0, y: 0 }, tr: { x: Math.max(0, 1 - o.width), y: 0 },
			bl: { x: 0, y: Math.max(0, 1 - o.height) }, br: { x: Math.max(0, 1 - o.width), y: Math.max(0, 1 - o.height) }
		};
		o.x = snap[corner].x; o.y = snap[corner].y;
		markDirty('overlay', id);
	}

	// ---------------------------------------------------------------------
	// Text handlers
	// ---------------------------------------------------------------------
	function handleTextDrag(id: number, posData: { x: number; y: number }) {
		const t = getText(id);
		if (!t) return;
		t.x = posData.x; t.y = posData.y;
		markDirty('text', id);
	}

	async function handleAddText() {
		// Date overlays derive their content at print time, so they need no text.
		if (newSource === TEXT_SOURCE_STATIC && !newText.trim()) return;
		await createTextOverlay(projectId, {
			text: newSource === TEXT_SOURCE_STATIC ? newText.trim() : '',
			font_family: newFontFamily || undefined,
			font_size: newFontSize,
			color: newTextColor,
			x: 0.5, y: 0.5, opacity: 1.0,
			orientation_id: orientation,
			source: newSource,
			date_format: isDateSource(newSource) ? (newDateFormat || defaultFormatFor(newSource)) : undefined,
			text_align: newAlign
		});
		newText = '';
		clearHistory();
		load();
	}

	async function handleTextDelete(id: number) {
		editingTextId = null;
		await deleteTextOverlay(id);
		clearHistory();
		load();
	}

	function startEditText(id: number) { editingTextId = editingTextId === id ? null : id; editingOverlayId = null; }
	function getText(id: number): TextOverlay | undefined { return data?.text_overlays?.find(x => x.id === id); }

	function updateTextProp(id: number, prop: string, value: string | number) {
		const t = getText(id);
		if (!t) return;
		(t as any)[prop] = value;
		markDirty('text', id);
	}

</script>

<svelte:window onkeydown={handleWindowKeydown} onbeforeunload={flushBeforeUnload} />

{#if !data}
	<p class="empty">Loading...</p>
{:else}
	<!-- Orientation Tabs -->
	<div class="orient-tabs">
		<button class="orient-tab" class:active={orientation === ORIENTATION_LANDSCAPE} onclick={() => orientation = ORIENTATION_LANDSCAPE}>
			Landscape
		</button>
		<button class="orient-tab" class:active={orientation === ORIENTATION_PORTRAIT} onclick={() => orientation = ORIENTATION_PORTRAIT}>
			Portrait
		</button>
		<button class="ghost copy-btn" onclick={handleCopyOrientation}>
			Copy to {orientation === ORIENTATION_LANDSCAPE ? 'Portrait' : 'Landscape'}
		</button>
	</div>

	<!-- Edit history + autosave status -->
	<div class="history-bar">
		<button class="ghost history-btn" disabled={!canUndo} onclick={() => void undo()} title="Undo (⌘Z)">
			↶ Undo
		</button>
		<button class="ghost history-btn" disabled={!canRedo} onclick={() => void redo()} title="Redo (⇧⌘Z)">
			↷ Redo
		</button>
		<span class="save-status" class:error={saveState === 'error'}>
			{#if saveState === 'saving'}Saving…
			{:else if saveState === 'pending'}Saving…
			{:else if saveState === 'error'}Save failed — check the connection
			{:else if canUndo}All changes saved
			{/if}
		</span>
	</div>

	<!-- Canvas Preview -->
	<section class="section">
		<h3>Preview ({orientation === ORIENTATION_LANDSCAPE ? 'Landscape' : 'Portrait'})</h3>
		<OverlayEditor
			overlays={filteredOverlays}
			textOverlays={filteredTextOverlays}
			{textPreview}
			{lockAspect}
			portrait={orientation === ORIENTATION_PORTRAIT}
			onOverlayUpdate={handleOverlayDrag}
			onTextUpdate={handleTextDrag}
		/>
	</section>

	<!-- Image Overlays -->
	<section class="section">
		<h3>Image Overlays</h3>
		{#each filteredOverlays as overlay (overlay.id)}
			<div class="item-card card">
				<div class="item-header">
					<span class="item-name">{overlay.filename}</span>
					<span class="item-meta">opacity: {overlay.opacity.toFixed(2)}</span>
					{#if editingOverlayId !== overlay.id}
						<button class="ghost sm-btn" onclick={() => startEditOverlay(overlay.id)}>Edit</button>
					{/if}
					<button class="ghost sm-btn danger-text" onclick={() => handleOverlayDelete(overlay.id)}>Remove</button>
				</div>
				{#if editingOverlayId === overlay.id}
					<div class="edit-inline">
						<div class="transform-grid">
							<NumberStepper label="X %" min={0} max={100} value={Math.round(overlay.x * 100)} onchange={(v) => updateOverlayProp(overlay.id, 'x', v / 100)} />
							<NumberStepper label="Y %" min={0} max={100} value={Math.round(overlay.y * 100)} onchange={(v) => updateOverlayProp(overlay.id, 'y', v / 100)} />
							<NumberStepper label="W %" min={1} max={100} value={Math.round(overlay.width * 100)} onchange={(v) => updateOverlayProp(overlay.id, 'width', v / 100)} />
							<NumberStepper label="H %" min={1} max={100} value={Math.round(overlay.height * 100)} onchange={(v) => updateOverlayProp(overlay.id, 'height', v / 100)} />
						</div>
						<div class="lock-row">
							<button class="lock-btn" class:locked={isLocked(overlay.id)} onclick={() => toggleLock(overlay.id)}>{isLocked(overlay.id) ? 'Uniform' : 'Free'}</button>
							<span class="lock-hint">{isLocked(overlay.id) ? 'Scales together' : 'Scales independently'}</span>
						</div>
						<label class="slider-group"><span>Opacity: {overlay.opacity.toFixed(2)}</span><input type="range" min="0" max="1" step="0.05" value={overlay.opacity} oninput={(e) => updateOverlayProp(overlay.id, 'opacity', Number((e.target as HTMLInputElement).value))} /></label>
						<div class="snap-row">
							<span class="snap-label">Snap:</span>
							<button class="snap-btn" onclick={() => snapOverlay(overlay.id, 'tl')}>TL</button>
							<button class="snap-btn" onclick={() => snapOverlay(overlay.id, 'tr')}>TR</button>
							<button class="snap-btn" onclick={() => snapOverlay(overlay.id, 'bl')}>BL</button>
							<button class="snap-btn" onclick={() => snapOverlay(overlay.id, 'br')}>BR</button>
						</div>
						<div class="edit-actions">
							<button class="ghost sm-btn" onclick={() => editingOverlayId = null}>Done</button>
						</div>
					</div>
				{/if}
			</div>
		{/each}
		<label class="upload-btn ghost">Upload Overlay PNG<input type="file" accept=".png" hidden onchange={handleOverlayUpload} /></label>
	</section>

	<!-- Text Overlays -->
	<section class="section">
		<h3>Text Overlays</h3>
		{#each filteredTextOverlays as t (t.id)}
			<div class="item-card card">
				<div class="item-header">
					<span class="item-name" style="color: {t.color};">{textPreview(t) || '(empty)'}</span>
					{#if isDateSource(t.source)}
						<span class="source-badge">{sourceLabel(t.source)}</span>
						<span class="item-meta">anchor: {(t.text_align || TEXT_ALIGN_LEFT)}</span>
					{/if}
					<span class="item-meta">{t.font_size}pt</span>
					{#if editingTextId !== t.id}<button class="ghost sm-btn" onclick={() => startEditText(t.id)}>Edit</button>{/if}
					<button class="ghost sm-btn danger-text" onclick={() => handleTextDelete(t.id)}>Remove</button>
				</div>
				{#if editingTextId === t.id}
					<div class="edit-inline">
						<label class="font-field">
							<span>Content</span>
							<select value={t.source} onchange={(e) => onEditSourceChange(t.id, (e.target as HTMLSelectElement).value as TextOverlaySource)} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem;">
								{#each sourceOptions as opt}
									<option value={opt.key}>{opt.label}</option>
								{/each}
							</select>
						</label>
						{#if isDateSource(t.source)}
							<label class="font-field">
								<span>Format</span>
								<select value={t.date_format || defaultFormatFor(t.source)} onchange={(e) => updateTextProp(t.id, 'date_format', (e.target as HTMLSelectElement).value)} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem;">
									{#each formatsFor(t.source) as f}
										<option value={f.key}>{f.example}</option>
									{/each}
								</select>
							</label>
							<p class="source-hint">
								Prints the photo's capture date. Photos without a capture date in
								their metadata fall back to the time they were uploaded.
							</p>
						{:else}
							<input type="text" value={t.text} oninput={(e) => updateTextProp(t.id, 'text', (e.target as HTMLInputElement).value)} />
						{/if}
						<label class="font-field">
							<span>Font</span>
							<select value={t.font_family || ''} onchange={(e) => updateTextProp(t.id, 'font_family', (e.target as HTMLSelectElement).value)} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem;">
								<option value="">System default</option>
								{#each fonts as f}
									<option value={f.path}>{f.name}</option>
								{/each}
							</select>
						</label>
						<div class="transform-grid">
							<NumberStepper label="X %" min={0} max={100} value={Math.round(t.x * 100)} onchange={(v) => updateTextProp(t.id, 'x', v / 100)} />
							<NumberStepper label="Y %" min={0} max={100} value={Math.round(t.y * 100)} onchange={(v) => updateTextProp(t.id, 'y', v / 100)} />
							<NumberStepper label="Size" min={8} max={400} value={t.font_size} onchange={(v) => updateTextProp(t.id, 'font_size', v)} />
							<label class="num-field"><span>Color</span><input type="color" value={t.color} oninput={(e) => updateTextProp(t.id, 'color', (e.target as HTMLInputElement).value)} style="height: 36px; padding: 2px; min-height: auto;" /></label>
						</div>
						<div class="align-row">
							<span class="align-label">Anchor</span>
							{#each ALIGN_OPTIONS as opt}
								<button
									type="button"
									class="align-btn"
									class:active={(t.text_align || TEXT_ALIGN_LEFT) === opt.value}
									onclick={() => updateTextProp(t.id, 'text_align', opt.value)}
								>{opt.label}</button>
							{/each}
						</div>
						<p class="source-hint">{alignHint(t.text_align)}</p>
						<label class="slider-group"><span>Opacity: {(t.opacity ?? 1).toFixed(2)}</span><input type="range" min="0" max="1" step="0.05" value={t.opacity ?? 1} oninput={(e) => updateTextProp(t.id, 'opacity', Number((e.target as HTMLInputElement).value))} /></label>
						<div class="edit-actions">
							<button class="ghost sm-btn" onclick={() => editingTextId = null}>Done</button>
						</div>
					</div>
				{/if}
			</div>
		{/each}
		<form class="add-text-form" onsubmit={(e) => { e.preventDefault(); handleAddText(); }}>
			<div class="text-options">
				<label>
					<span>Content</span>
					<select value={newSource} onchange={(e) => onNewSourceChange((e.target as HTMLSelectElement).value as TextOverlaySource)} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem; max-width: 220px;">
						{#each sourceOptions as opt}
							<option value={opt.key}>{opt.label}</option>
						{/each}
					</select>
				</label>
				{#if isDateSource(newSource)}
					<label>
						<span>Format</span>
						<select bind:value={newDateFormat} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem; max-width: 220px;">
							{#each formatsFor(newSource) as f}
								<option value={f.key}>{f.example}</option>
							{/each}
						</select>
					</label>
				{/if}
			</div>
			{#if isDateSource(newSource)}
				<p class="source-hint">
					Prints the photo's capture date. Photos without a capture date in their
					metadata fall back to the time they were uploaded.
				</p>
			{:else}
				<input type="text" placeholder="Text content" bind:value={newText} />
			{/if}
			<div class="text-options">
				<label>
					<span>Font</span>
					<select bind:value={newFontFamily} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem; max-width: 160px;">
						<option value="">System default</option>
						{#each fonts as f}
							<option value={f.path}>{f.name}</option>
						{/each}
					</select>
				</label>
				<label><span>Size</span><input type="number" min="8" max="200" bind:value={newFontSize} style="width: 70px;" /></label>
				<label><span>Color</span><input type="color" bind:value={newTextColor} style="width: 50px; height: 36px; padding: 2px; min-height: auto;" /></label>
				<label>
					<span>Anchor</span>
					<select bind:value={newAlign} style="min-height: auto; padding: 4px 8px; font-size: 0.8rem;">
						{#each ALIGN_OPTIONS as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</label>
			</div>
			<p class="source-hint">{alignHint(newAlign)}</p>
			<button class="primary" type="submit" style="padding: 8px 16px;">{isDateSource(newSource) ? 'Add Date' : 'Add Text'}</button>
		</form>
	</section>
{/if}

<style>
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }

	.orient-tabs {
		display: flex;
		align-items: center;
		gap: 0;
		margin-bottom: 20px;
		border-bottom: 1px solid var(--border);
	}

	.orient-tab {
		padding: 10px 20px;
		background: none;
		color: var(--text-muted);
		font-weight: 500;
		font-size: 0.875rem;
		border-bottom: 2px solid transparent;
		border-radius: 0;
		min-height: 44px;
	}

	.orient-tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.copy-btn {
		margin-left: auto;
		padding: 6px 14px;
		font-size: 0.75rem;
		min-height: auto;
	}

	.section { margin-bottom: 28px; }

	.source-badge {
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		font-weight: 600;
		padding: 2px 6px;
		border-radius: 4px;
		background: var(--accent);
		color: #fff;
		white-space: nowrap;
	}

	.align-row {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.align-label {
		font-size: 0.75rem;
		color: var(--text-muted);
		min-width: 52px;
	}

	.align-btn {
		padding: 0 14px;
		font-size: 0.8rem;
		min-height: 34px;
		touch-action: manipulation;
		background: none;
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text-muted);
	}

	.align-btn.active {
		border-color: var(--accent);
		color: var(--accent);
	}

	.source-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin: 0;
		line-height: 1.4;
	}
	.section h3 { font-size: 1rem; font-weight: 600; margin-bottom: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
	.item-card { padding: 12px 16px; margin-bottom: 8px; }
	.item-header { display: flex; align-items: center; gap: 8px; }
	.item-name { flex: 1; font-size: 0.875rem; font-weight: 500; }
	.item-meta { color: var(--text-muted); font-size: 0.75rem; }
	.sm-btn { padding: 4px 10px; font-size: 0.8rem; }
	.danger-text { color: var(--danger); }
	.edit-inline { display: flex; flex-direction: column; gap: 10px; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border); }
	.edit-actions { display: flex; gap: 8px; }

	.history-bar {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 14px;
	}

	.history-btn {
		padding: 0 14px;
		min-height: 34px;
		font-size: 0.8rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		touch-action: manipulation;
	}

	.history-btn:disabled {
		opacity: 0.35;
		cursor: default;
	}

	.save-status {
		font-size: 0.75rem;
		color: var(--text-muted);
		margin-left: auto;
	}

	.save-status.error { color: var(--danger, #ff6b6b); }
	.transform-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(116px, 1fr)); gap: 8px; }
	.num-field { display: flex; flex-direction: column; gap: 3px; }
	.num-field span { font-size: 0.7rem; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
	.num-field input { padding: 0 4px; font-size: 0.8rem; min-height: 34px; width: 100%; text-align: center; }
	.lock-row { display: flex; align-items: center; gap: 8px; }
	.lock-btn { padding: 0 14px; font-size: 0.8rem; min-height: 34px; min-width: auto; border: 1px solid var(--border); border-radius: 4px; background: var(--bg-elevated); color: var(--text-muted); touch-action: manipulation; }
	.lock-btn.locked { border-color: var(--accent); color: var(--accent); background: rgba(74, 158, 255, 0.1); }
	.lock-hint { font-size: 0.7rem; color: var(--text-muted); }
	.slider-group { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
	.slider-group span { font-size: 0.8rem; color: var(--text-muted); }
	.slider-group input[type="range"] { width: 100%; min-height: auto; padding: 0; border: none; background: transparent; }
	.snap-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
	.snap-label { font-size: 0.8rem; color: var(--text-muted); }
	.snap-btn { padding: 0 14px; font-size: 0.8rem; background: var(--bg-elevated); color: var(--text-muted); border: 1px solid var(--border); border-radius: 4px; min-height: 34px; min-width: 44px; touch-action: manipulation; }
	.snap-btn:hover { border-color: var(--accent); color: var(--accent); }
	.upload-btn { display: inline-block; padding: 8px 16px; font-size: 0.875rem; cursor: pointer; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-muted); margin-top: 8px; }
	.font-field { display: flex; flex-direction: column; gap: 2px; }
	.font-field span { font-size: 0.75rem; color: var(--text-muted); }
	.add-text-form { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
	.text-options { display: flex; gap: 12px; align-items: flex-end; }
	.text-options label { display: flex; flex-direction: column; gap: 2px; }
	.text-options label span { font-size: 0.75rem; color: var(--text-muted); }
</style>
