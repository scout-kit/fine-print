<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import {
		previewUrl, downloadOriginalUrl, downloadRenderedUrl, renderPreviewUrl,
		photoStatusName, approvePhoto, rejectPhoto, unapprovePhoto, deletePhoto, reprintPhoto
	} from '$lib/api';
	import { isAdmin } from '$lib/stores';

	// Accepts any object with these fields — works with Photo and GalleryPhoto
	interface ModalPhoto {
		id: number;
		project_id?: number;
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
		projectName?: string;
	}

	let { photo, onClose, onAction, guestSession = '', projectName = '' }: Props = $props();
	let showMore = $state(false);

	let admin = $state(false);
	isAdmin.subscribe(v => admin = v);

	let showRendered = $state(true); // default to print preview
	let acting = $state(false);
	let renderTs = $state(Date.now());

	// Refresh render timestamp when navigating back (e.g. after editing)
	afterNavigate(() => { renderTs = Date.now(); });

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
				<img src={renderPreviewUrl(photo.id) + '?t=' + renderTs} alt="Print Preview" />
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
			{#if projectName}
				<span class="meta project-name">{projectName}</span>
			{/if}
			{#if (photo.copies || 1) > 1}
				<span class="meta copies-badge">{photo.copies}x</span>
			{/if}
			<span class="meta" style="margin-left:auto">{new Date(photo.created_at).toLocaleTimeString()}</span>
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
			<div class="actions-primary">
				{#if admin && photo.status_id === 1}
					<button class="act-btn approve" onclick={handleApprove} disabled={acting}>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
						Approve
					</button>
					<button class="act-btn reject" onclick={handleReject} disabled={acting}>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
						Reject
					</button>
				{/if}

				<a href="/edit?id={photo.id}" class="act-btn">
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
					Edit
				</a>

				<button class="act-btn" onclick={() => showMore = !showMore}>
					<svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor"><circle cx="12" cy="5" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="12" cy="19" r="1.5"/></svg>
					More
				</button>
			</div>

			{#if showMore}
				<div class="actions-more">
					<a href={downloadOriginalUrl(photo.id)} class="more-item" download>Download Original</a>
					<a href={renderPreviewUrl(photo.id)} class="more-item" download="print_{photo.id}.jpg">Download Print</a>

					{#if admin && (photo.status_id === 2 || photo.status_id === 7)}
						<button class="more-item" onclick={() => act(() => unapprovePhoto(photo.id))} disabled={acting}>Unapprove</button>
					{/if}

					{#if admin && (photo.status_id === 5 || photo.status_id === 6)}
						<button class="more-item" onclick={() => act(() => reprintPhoto(photo.id, false))} disabled={acting}>Reprint</button>
						<button class="more-item" onclick={() => act(() => reprintPhoto(photo.id, true))} disabled={acting}>Reprint (fresh render)</button>
					{/if}

					{#if admin || (isOwn && photo.status_id !== 4)}
						<button class="more-item danger-text" onclick={handleDelete} disabled={acting}>Delete</button>
					{/if}
				</div>
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
		padding: 0;
	}

	.actions-primary {
		display: flex;
		gap: 6px;
		padding: 10px 16px;
	}

	.act-btn {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 7px 14px;
		font-size: 0.8rem;
		font-weight: 600;
		border-radius: 8px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text-muted);
		text-decoration: none;
		cursor: pointer;
		min-height: auto;
		min-width: auto;
		white-space: nowrap;
	}

	.act-btn:hover { border-color: var(--accent); color: var(--text); }
	.act-btn.approve { background: var(--success); color: #000; border-color: var(--success); }
	.act-btn.approve:hover { opacity: 0.85; }
	.act-btn.reject { background: var(--danger); color: white; border-color: var(--danger); }
	.act-btn.reject:hover { opacity: 0.85; }

	.actions-more {
		border-top: 1px solid var(--border);
		padding: 6px 0;
	}

	.more-item {
		display: block;
		width: 100%;
		padding: 8px 16px;
		font-size: 0.8rem;
		color: var(--text-muted);
		background: none;
		border: none;
		text-align: left;
		cursor: pointer;
		text-decoration: none;
		font-family: inherit;
	}

	.more-item:hover { background: var(--bg-elevated); color: var(--text); }

	.project-name {
		font-weight: 500;
		color: var(--accent);
	}

	.danger-text { color: var(--danger); }
</style>
