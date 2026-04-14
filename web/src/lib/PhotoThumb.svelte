<script lang="ts">
	import { renderPreviewUrl, previewUrl, photoStatusName } from '$lib/api';

	interface ThumbPhoto {
		id: number;
		status_id: number;
		preview_key?: string | null;
		has_preview?: boolean;
	}

	interface Props {
		photo: ThumbPhoto;
		onclick: () => void;
		showProject?: string;
		selectable?: boolean;
		selected?: boolean;
	}

	let { photo, onclick, showProject = '', selectable = false, selected = false }: Props = $props();

	const hasPreview = $derived(!!(photo.preview_key || photo.has_preview));
</script>

<button class="thumb" class:selected={selectable && selected} {onclick}>
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
</button>

<style>
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
