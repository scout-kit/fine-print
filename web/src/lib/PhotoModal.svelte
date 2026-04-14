<script lang="ts">
	import {
		previewUrl, downloadOriginalUrl, downloadRenderedUrl, renderPreviewUrl,
		photoStatusName, approvePhoto, rejectPhoto, unapprovePhoto, deletePhoto, reprintPhoto
	} from '$lib/api';
	import { isAdmin } from '$lib/stores';

	// Accepts any object with these fields — works with Photo and GalleryPhoto
	interface ModalPhoto {
		id: number;
		status_id: number;
		session_id?: string;
		preview_key?: string | null;
		has_preview?: boolean;
		copies?: number;
		created_at: string;
	}

	interface Props {
		photo: ModalPhoto;
		onClose: () => void;
		onAction: () => void;
		guestSession?: string;
	}

	let { photo, onClose, onAction, guestSession = '' }: Props = $props();

	let admin = $state(false);
	isAdmin.subscribe(v => admin = v);

	let showRendered = $state(true); // default to print preview
	let acting = $state(false);

	const status = $derived(photoStatusName(photo.status_id));
	const isOwn = $derived(guestSession !== '' && photo.session_id === guestSession);
	const hasPreview = $derived(!!(photo.preview_key || photo.has_preview));

	async function act(fn: () => Promise<unknown>) {
		acting = true;
		try {
			await fn();
			onAction();
			onClose();
		} catch (e) {
			console.error(e);
		}
		acting = false;
	}

	function handleApprove() { act(() => approvePhoto(photo.id)); }
	function handleReject() { act(() => rejectPhoto(photo.id)); }
	function handleDelete() {
		if (!confirm('Delete this photo?')) return;
		act(() => deletePhoto(photo.id));
	}
</script>

<div class="backdrop" onclick={onClose} role="button" tabindex="-1">
	<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
		<button class="close-btn" onclick={onClose}>&times;</button>

		<!-- Image -->
		<div class="image-area">
			{#if showRendered}
				<img src={renderPreviewUrl(photo.id)} alt="Print Preview" />
				<span class="image-label">Print Preview</span>
			{:else if hasPreview}
				<img src={previewUrl(photo.id)} alt="Original" />
				<span class="image-label">Original</span>
			{:else}
				<div class="no-image">Preview not available</div>
			{/if}
		</div>

		<!-- Info bar -->
		<div class="info-bar">
			<span class="badge {status}">{status}</span>
			<span class="meta">#{photo.id}</span>
			{#if (photo.copies || 1) > 1}
				<span class="meta copies-badge">{photo.copies} copies</span>
			{/if}
			<span class="meta">{new Date(photo.created_at).toLocaleString()}</span>
		</div>

		<!-- Image toggle -->
		{#if hasPreview}
			<div class="toggle-row">
				<button
					class="toggle-btn" class:active={!showRendered}
					onclick={() => showRendered = false}
				>Original</button>
				<button
					class="toggle-btn" class:active={showRendered}
					onclick={() => showRendered = true}
				>Print Preview</button>
			</div>
		{/if}

		<!-- Actions -->
		<div class="actions">
			{#if admin}
				{#if photo.status_id === 1}
					<button class="success action-btn" onclick={handleApprove} disabled={acting}>Approve</button>
					<button class="danger action-btn" onclick={handleReject} disabled={acting}>Reject</button>
				{/if}

				{#if photo.status_id === 2 || photo.status_id === 7}
					<button class="ghost action-btn" onclick={() => act(() => unapprovePhoto(photo.id))} disabled={acting}>Unapprove</button>
				{/if}

				<a href="/edit?id={photo.id}" class="ghost action-btn">Edit</a>
				<a href={downloadOriginalUrl(photo.id)} class="ghost action-btn" download>Download Original</a>
				<a href={renderPreviewUrl(photo.id)} class="ghost action-btn" download="print_{photo.id}.jpg">Download Print</a>

				{#if photo.status_id === 5 || photo.status_id === 6}
					<button class="ghost action-btn" onclick={() => act(() => reprintPhoto(photo.id, false))} disabled={acting}>Reprint</button>
					<button class="ghost action-btn" onclick={() => act(() => reprintPhoto(photo.id, true))} disabled={acting}>Reprint (fresh)</button>
				{/if}

				<button class="ghost action-btn danger-text" onclick={handleDelete} disabled={acting}>Delete</button>

			{:else}
				<a href="/edit?id={photo.id}" class="ghost action-btn">Edit</a>
				<a href={downloadOriginalUrl(photo.id)} class="ghost action-btn" download>Download Original</a>
				<a href={renderPreviewUrl(photo.id)} class="ghost action-btn" download="print_{photo.id}.jpg">Download Print</a>

				{#if isOwn && photo.status_id !== 4}
					<button class="ghost action-btn danger-text" onclick={handleDelete} disabled={acting}>Delete</button>
				{/if}
			{/if}
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.88);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 100;
		padding: 16px;
	}

	.modal {
		background: var(--bg-surface);
		border-radius: var(--radius);
		max-width: 440px;
		width: 100%;
		overflow: hidden;
		position: relative;
		max-height: 90vh;
		overflow-y: auto;
	}

	.close-btn {
		position: absolute;
		top: 8px;
		right: 8px;
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background: rgba(0, 0, 0, 0.6);
		color: white;
		font-size: 1.3rem;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		min-height: auto;
		min-width: auto;
		z-index: 1;
	}

	.image-area {
		width: 100%;
		min-height: 200px;
		background: #000;
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.image-area img {
		width: 100%;
		max-height: 50vh;
		object-fit: contain;
	}

	.image-label {
		position: absolute;
		top: 8px;
		left: 8px;
		font-size: 0.65rem;
		padding: 2px 8px;
		border-radius: 4px;
		background: rgba(0, 0, 0, 0.6);
		color: #aaa;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.no-image {
		padding: 48px;
		color: var(--text-muted);
		font-size: 0.875rem;
	}

	.info-bar {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 10px 16px;
		border-bottom: 1px solid var(--border);
	}

	.meta {
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	.copies-badge {
		background: var(--bg-elevated);
		padding: 1px 6px;
		border-radius: 4px;
		font-weight: 600;
	}

	.toggle-row {
		display: flex;
		border-bottom: 1px solid var(--border);
	}

	.toggle-btn {
		flex: 1;
		padding: 8px;
		background: none;
		color: var(--text-muted);
		font-size: 0.8rem;
		font-weight: 500;
		border-radius: 0;
		border-bottom: 2px solid transparent;
		min-height: auto;
	}

	.toggle-btn.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		padding: 12px 16px;
	}

	.action-btn {
		padding: 8px 16px;
		font-size: 0.85rem;
		border-radius: var(--radius-sm);
		text-decoration: none;
		text-align: center;
		font-weight: 500;
		min-height: 38px;
		display: inline-flex;
		align-items: center;
	}

	a.action-btn {
		border: 1px solid var(--border);
		color: var(--text-muted);
	}

	.danger-text { color: var(--danger); }
</style>
