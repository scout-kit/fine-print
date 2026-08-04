<script lang="ts">
	import { renderPreviewUrl, previewUrl, photoStatusName } from '$lib/api';
	import { formatTakenAtShort, isExifDate } from '$lib/photometa';

	interface ThumbPhoto {
		id: number;
		status_id: number;
		preview_key?: string | null;
		has_preview?: boolean;
		created_at?: string;
		taken_at?: string;
		taken_at_source?: 'exif' | 'upload';
	}

	interface Props {
		photo: ThumbPhoto;
		onclick: () => void;
		onlongpress?: () => void;
		showProject?: string;
		selectable?: boolean;
		selected?: boolean;
		/** Show the capture date under the thumbnail. */
		showTakenAt?: boolean;
	}

	let { photo, onclick, onlongpress, showProject = '', selectable = false, selected = false, showTakenAt = false }: Props = $props();

	const hasPreview = $derived(!!(photo.preview_key || photo.has_preview));
	// Only render a date when the photo actually carries a timestamp.
	const takenAtLabel = $derived(
		showTakenAt && (photo.taken_at || photo.created_at)
			? formatTakenAtShort({ created_at: photo.created_at ?? '', taken_at: photo.taken_at, taken_at_source: photo.taken_at_source })
			: ''
	);
	const takenAtIsExif = $derived(isExifDate({ created_at: photo.created_at ?? '', taken_at_source: photo.taken_at_source }));

	let pressTimer: ReturnType<typeof setTimeout> | null = null;
	let didLongPress = false;

	function startPress() {
		didLongPress = false;
		pressTimer = setTimeout(() => {
			didLongPress = true;
			onlongpress?.();
		}, 500);
	}

	function endPress() {
		if (pressTimer) { clearTimeout(pressTimer); pressTimer = null; }
	}

	function handleClick() {
		if (didLongPress) { didLongPress = false; return; }
		onclick();
	}
</script>

<button
	class="thumb" class:selected={selectable && selected}
	onclick={handleClick}
	onpointerdown={startPress}
	onpointerup={endPress}
	onpointerleave={endPress}
	oncontextmenu={(e) => { if (onlongpress) e.preventDefault(); }}
>
	<div class="image">
		{#if hasPreview}
			<img src={renderPreviewUrl(photo.id)} alt="Photo {photo.id}" loading="lazy" />
		{:else}
			<div class="no-preview">Processing</div>
		{/if}
		<span class="badge {photoStatusName(photo.status_id)}">{photoStatusName(photo.status_id)}</span>
		{#if selectable}
			<span class="check" class:checked={selected}>
				{#if selected}
					<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="3"><polyline points="20 6 9 17 4 12"/></svg>
				{/if}
			</span>
		{/if}
	</div>
	{#if showProject}
		<span class="project-label">{showProject}</span>
	{/if}
	{#if takenAtLabel}
		<span
			class="taken-label"
			title={takenAtIsExif ? 'Capture date from photo metadata' : 'No capture date in file — showing upload time'}
		>{takenAtLabel}{#if !takenAtIsExif}<span class="approx">~</span>{/if}</span>
	{/if}
</button>

<style>
	.taken-label {
		display: block;
		padding: 0 6px 5px;
		font-size: 0.68rem;
		color: var(--text-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.approx { margin-left: 2px; opacity: 0.7; font-weight: 600; }

	.thumb {
		background: var(--bg-surface);
		border: 2px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
		padding: 0;
		cursor: pointer;
		text-align: left;
		min-height: auto;
		min-width: auto;
		transition: border-color 0.15s;
		-webkit-touch-callout: none;
		-webkit-user-select: none;
		user-select: none;
	}

	.thumb:hover {
		border-color: var(--accent);
	}

	.thumb.selected {
		border-color: var(--accent);
	}

	.image {
		aspect-ratio: 1;
		position: relative;
		background: #000;
	}

	.image img {
		width: 100%;
		height: 100%;
		object-fit: contain;
		pointer-events: none;
	}

	.image .badge {
		position: absolute;
		bottom: 4px;
		left: 4px;
		font-size: 0.6rem;
	}

	.check {
		position: absolute;
		top: 4px;
		right: 4px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		border: 2px solid rgba(255, 255, 255, 0.7);
		background: rgba(0, 0, 0, 0.3);
		display: flex;
		align-items: center;
		justify-content: center;
		color: white;
	}

	.check.checked {
		background: var(--accent);
		border-color: var(--accent);
	}

	.no-preview {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-muted);
		font-size: 0.7rem;
	}

	.project-label {
		display: block;
		padding: 3px 6px;
		font-size: 0.6rem;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}
</style>
