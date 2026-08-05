<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import OverlayEditor from '$lib/OverlayEditor.svelte';
	import {
		getProject, uploadOverlay, updateOverlayPosition, deleteOverlay,
		createTextOverlay, updateTextOverlay, deleteTextOverlay,
		copyTemplateOrientation, listAvailableFonts, listDateFormats,
		ORIENTATION_LANDSCAPE, ORIENTATION_PORTRAIT,
		TEXT_SOURCE_STATIC, TEXT_SOURCE_PHOTO_DATE, TEXT_SOURCE_PHOTO_DATETIME,
		TEXT_ALIGN_LEFT, TEXT_ALIGN_RIGHT,
		type ProjectResponse, type Overlay, type TextOverlay, type SystemFont,
		type TextSourceOption, type TextOverlaySource, type DateFormatOption,
		type TextAlign
	} from '$lib/api';

	const projectId = $derived(Number(page.params.id));
	let data: ProjectResponse | null = $state(null);
	let editorVersion = $state(0);
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
		{ value: TEXT_ALIGN_LEFT,  label: 'Left',  hint: 'x is the left edge — text grows right' },
		{ value: TEXT_ALIGN_RIGHT, label: 'Right', hint: 'x is the right edge — text grows left' }
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
			editorVersion++;
		} catch { data = null; }
	}

	onMount(async () => {
		load();
		try { fonts = await listAvailableFonts(); } catch { /* ignore */ }
		try { sourceOptions = (await listDateFormats()).sources; } catch { /* ignore */ }
	});

	function refreshEditor() { editorVersion++; }

	// Overlay handlers
	async function handleOverlayUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.[0]) return;
		await uploadOverlay(projectId, input.files[0], orientation);
		input.value = '';
		load();
	}

	async function handleCopyOrientation() {
		const from = orientation;
		const to = orientation === ORIENTATION_LANDSCAPE ? ORIENTATION_PORTRAIT : ORIENTATION_LANDSCAPE;
		await copyTemplateOrientation(projectId, from, to);
		load();
	}

	async function handleOverlayDrag(id: number, posData: { x: number; y: number; width: number; height: number; opacity: number }) {
		if (data?.overlays) {
			const o = data.overlays.find(x => x.id === id);
			if (o) { o.x = posData.x; o.y = posData.y; o.width = posData.width; o.height = posData.height; o.opacity = posData.opacity; }
		}
		await updateOverlayPosition(id, posData);
	}

	async function handleOverlayDelete(id: number) {
		editingOverlayId = null;
		await deleteOverlay(id);
		load();
	}

	function startEditOverlay(id: number) { editingOverlayId = id; editingTextId = null; }
	function getOverlay(id: number): Overlay | undefined { return data?.overlays?.find(x => x.id === id); }
	function isLocked(id: number): boolean { return lockAspect[id] !== false; }
	function toggleLock(id: number) { lockAspect = { ...lockAspect, [id]: !isLocked(id) }; refreshEditor(); }

	function updateOverlayProp(id: number, prop: 'x' | 'y' | 'width' | 'height' | 'opacity', value: number) {
		if (!data?.overlays) return;
		const o = data.overlays.find(x => x.id === id);
		if (!o) return;
		if (prop === 'width' && isLocked(id) && o.width > 0) { o.height = Math.max(0.01, Math.min(1, o.height * (value / o.width))); }
		else if (prop === 'height' && isLocked(id) && o.height > 0) { o.width = Math.max(0.01, Math.min(1, o.width * (value / o.height))); }
		(o as any)[prop] = Math.max(0, Math.min(1, value));
		refreshEditor();
	}

	async function saveOverlay(id: number) {
		const o = getOverlay(id);
		if (!o) return;
		await updateOverlayPosition(id, { x: o.x, y: o.y, width: o.width, height: o.height, opacity: o.opacity });
		editingOverlayId = null;
	}

	async function snapOverlay(id: number, corner: 'tl' | 'tr' | 'bl' | 'br') {
		const o = getOverlay(id);
		if (!o) return;
		const snap: Record<string, { x: number; y: number }> = {
			tl: { x: 0, y: 0 }, tr: { x: Math.max(0, 1 - o.width), y: 0 },
			bl: { x: 0, y: Math.max(0, 1 - o.height) }, br: { x: Math.max(0, 1 - o.width), y: Math.max(0, 1 - o.height) }
		};
		o.x = snap[corner].x; o.y = snap[corner].y;
		await updateOverlayPosition(id, { x: o.x, y: o.y, width: o.width, height: o.height, opacity: o.opacity });
		await load();
	}

	// Text handlers
	async function handleTextDrag(id: number, posData: { x: number; y: number }) {
		await updateTextOverlay(id, posData);
		if (data?.text_overlays) { const t = data.text_overlays.find(x => x.id === id); if (t) { t.x = posData.x; t.y = posData.y; } }
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
		load();
	}

	async function handleTextDelete(id: number) { editingTextId = null; await deleteTextOverlay(id); load(); }
	function startEditText(id: number) { editingTextId = id; editingOverlayId = null; }
	function getText(id: number): TextOverlay | undefined { return data?.text_overlays?.find(x => x.id === id); }

	function updateTextProp(id: number, prop: string, value: string | number) {
		if (data?.text_overlays) { const t = data.text_overlays.find(x => x.id === id); if (t) { (t as any)[prop] = value; refreshEditor(); } }
	}

	async function saveText(id: number) {
		const t = getText(id);
		if (!t) return;
		await updateTextOverlay(id, { text: t.text, font_family: t.font_family, font_size: t.font_size, color: t.color, x: t.x, y: t.y, opacity: t.opacity, source: t.source, date_format: t.date_format, text_align: t.text_align });
		editingTextId = null;
	}
</script>

{#if !data}
	<p class="empty">Loading...</p>
{:else}
	<!-- Orientation Tabs -->
	<div class="orient-tabs">
		<button class="orient-tab" class:active={orientation === ORIENTATION_LANDSCAPE} onclick={() => { orientation = ORIENTATION_LANDSCAPE; editorVersion++; }}>
			Landscape
		</button>
		<button class="orient-tab" class:active={orientation === ORIENTATION_PORTRAIT} onclick={() => { orientation = ORIENTATION_PORTRAIT; editorVersion++; }}>
			Portrait
		</button>
		<button class="ghost copy-btn" onclick={handleCopyOrientation}>
			Copy to {orientation === ORIENTATION_LANDSCAPE ? 'Portrait' : 'Landscape'}
		</button>
	</div>

	<!-- Canvas Preview -->
	<section class="section">
		<h3>Preview ({orientation === ORIENTATION_LANDSCAPE ? 'Landscape' : 'Portrait'})</h3>
		{#key editorVersion}
			<OverlayEditor
				overlays={filteredOverlays}
				textOverlays={filteredTextOverlays}
				{textPreview}
				{lockAspect}
				portrait={orientation === ORIENTATION_PORTRAIT}
				onOverlayUpdate={handleOverlayDrag}
				onTextUpdate={handleTextDrag}
			/>
		{/key}
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
							<label class="num-field"><span>X %</span><input type="number" min="0" max="100" step="1" value={Math.round(overlay.x * 100)} oninput={(e) => updateOverlayProp(overlay.id, 'x', Number((e.target as HTMLInputElement).value) / 100)} /></label>
							<label class="num-field"><span>Y %</span><input type="number" min="0" max="100" step="1" value={Math.round(overlay.y * 100)} oninput={(e) => updateOverlayProp(overlay.id, 'y', Number((e.target as HTMLInputElement).value) / 100)} /></label>
							<label class="num-field"><span>W %</span><input type="number" min="1" max="100" step="1" value={Math.round(overlay.width * 100)} oninput={(e) => updateOverlayProp(overlay.id, 'width', Number((e.target as HTMLInputElement).value) / 100)} /></label>
							<label class="num-field"><span>H %</span><input type="number" min="1" max="100" step="1" value={Math.round(overlay.height * 100)} oninput={(e) => updateOverlayProp(overlay.id, 'height', Number((e.target as HTMLInputElement).value) / 100)} /></label>
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
							<button class="primary sm-btn" onclick={() => saveOverlay(overlay.id)}>Save</button>
							<button class="ghost sm-btn" onclick={() => { editingOverlayId = null; load(); }}>Cancel</button>
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
							<label class="num-field"><span>X %</span><input type="number" min="0" max="100" step="1" value={Math.round(t.x * 100)} oninput={(e) => updateTextProp(t.id, 'x', Number((e.target as HTMLInputElement).value) / 100)} /></label>
							<label class="num-field"><span>Y %</span><input type="number" min="0" max="100" step="1" value={Math.round(t.y * 100)} oninput={(e) => updateTextProp(t.id, 'y', Number((e.target as HTMLInputElement).value) / 100)} /></label>
							<label class="num-field"><span>Size</span><input type="number" min="8" max="400" step="1" value={t.font_size} oninput={(e) => updateTextProp(t.id, 'font_size', Number((e.target as HTMLInputElement).value))} /></label>
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
							<button class="primary sm-btn" onclick={() => saveText(t.id)}>Save</button>
							<button class="ghost sm-btn" onclick={() => { editingTextId = null; load(); }}>Cancel</button>
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
		padding: 4px 12px;
		font-size: 0.75rem;
		min-height: auto;
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
	.transform-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px; }
	.num-field { display: flex; flex-direction: column; gap: 2px; }
	.num-field span { font-size: 0.7rem; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; }
	.num-field input { padding: 6px 8px; font-size: 0.8rem; min-height: auto; width: 100%; text-align: center; }
	.lock-row { display: flex; align-items: center; gap: 8px; }
	.lock-btn { padding: 4px 12px; font-size: 0.75rem; min-height: auto; min-width: auto; border: 1px solid var(--border); border-radius: 4px; background: var(--bg-elevated); color: var(--text-muted); }
	.lock-btn.locked { border-color: var(--accent); color: var(--accent); background: rgba(74, 158, 255, 0.1); }
	.lock-hint { font-size: 0.7rem; color: var(--text-muted); }
	.slider-group { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
	.slider-group span { font-size: 0.8rem; color: var(--text-muted); }
	.slider-group input[type="range"] { width: 100%; min-height: auto; padding: 0; border: none; background: transparent; }
	.snap-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
	.snap-label { font-size: 0.8rem; color: var(--text-muted); }
	.snap-btn { padding: 4px 10px; font-size: 0.75rem; background: var(--bg-elevated); color: var(--text-muted); border: 1px solid var(--border); border-radius: 4px; min-height: auto; min-width: auto; }
	.snap-btn:hover { border-color: var(--accent); color: var(--accent); }
	.upload-btn { display: inline-block; padding: 8px 16px; font-size: 0.875rem; cursor: pointer; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-muted); margin-top: 8px; }
	.font-field { display: flex; flex-direction: column; gap: 2px; }
	.font-field span { font-size: 0.75rem; color: var(--text-muted); }
	.add-text-form { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
	.text-options { display: flex; gap: 12px; align-items: flex-end; }
	.text-options label { display: flex; flex-direction: column; gap: 2px; }
	.text-options label span { font-size: 0.75rem; color: var(--text-muted); }
</style>
