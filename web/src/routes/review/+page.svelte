<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { previewUrl, photoStatus } from '$lib/api';

	const ids = $derived(
		(page.url.searchParams.get('ids') || '')
			.split(',')
			.map(Number)
			.filter(n => n > 0)
	);

	interface ReviewPhoto {
		id: number;
		ready: boolean;
		edited: boolean;
	}

	let photos: ReviewPhoto[] = $state([]);
	let currentIndex = $state(0);

	onMount(() => {
		photos = ids.map(id => ({ id, ready: false, edited: false }));
		pollPreviews();
	});

	async function pollPreviews() {
		for (let attempt = 0; attempt < 30; attempt++) {
			let allReady = true;
			for (const photo of photos) {
				if (!photo.ready) {
					try {
						const res = await fetch(previewUrl(photo.id), { method: 'HEAD' });
						if (res.ok) photo.ready = true;
						else allReady = false;
					} catch {
						allReady = false;
					}
				}
			}
			photos = [...photos]; // trigger reactivity
			if (allReady) return;
			await new Promise(r => setTimeout(r, 1500));
		}
	}

	function editPhoto(id: number) {
		// Navigate to crop, with a return URL
		goto(`/edit?id=${id}&return=${encodeURIComponent(page.url.pathname + page.url.search)}`);
	}

	function markEdited(id: number) {
		const p = photos.find(x => x.id === id);
		if (p) p.edited = true;
		photos = [...photos];
	}

	function done() {
		goto('/gallery');
	}

	let editedCount = $derived(photos.filter(p => p.edited).length);
</script>

<div class="container">
	<header>
		<h2>Review Your Photos</h2>
		<p class="subtitle">{photos.length} photo{photos.length !== 1 ? 's' : ''} uploaded{editedCount > 0 ? `, ${editedCount} edited` : ''}</p>
	</header>

	<div class="photo-list">
		{#each photos as photo, i (photo.id)}
			<div class="review-item card">
				<div class="thumb">
					{#if photo.ready}
						<img src={previewUrl(photo.id)} alt="Photo {i + 1}" />
					{:else}
						<div class="loading">
							<div class="spinner-sm"></div>
						</div>
					{/if}
				</div>
				<div class="item-info">
					<span class="item-num">Photo {i + 1}</span>
					{#if photo.edited}
						<span class="edited-badge">Edited</span>
					{/if}
				</div>
				<div class="item-actions">
					{#if photo.ready}
						<button class="ghost action-btn" onclick={() => editPhoto(photo.id)}>
							Edit
						</button>
					{:else}
						<span class="processing">Processing...</span>
					{/if}
				</div>
			</div>
		{/each}
	</div>

	<div class="bottom-actions">
		<button class="primary done-btn" onclick={done}>
			Done
		</button>
		<p class="done-hint">Photos will be reviewed by the admin before printing</p>
	</div>
</div>

<style>
	header {
		padding: 24px 0 16px;
	}

	h2 { font-size: 1.5rem; font-weight: 700; }

	.subtitle {
		color: var(--text-muted);
		font-size: 0.875rem;
		margin-top: 4px;
	}

	.photo-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.review-item {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
	}

	.thumb {
		width: 80px;
		height: 54px;
		border-radius: 6px;
		overflow: hidden;
		flex-shrink: 0;
		background: var(--bg-elevated);
	}

	.thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.loading {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
	}

	.spinner-sm {
		width: 20px;
		height: 20px;
		border: 2px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin { to { transform: rotate(360deg); } }

	.item-info {
		flex: 1;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.item-num {
		font-size: 0.9rem;
		font-weight: 500;
	}

	.edited-badge {
		font-size: 0.65rem;
		padding: 2px 8px;
		border-radius: 999px;
		background: #143d1f;
		color: var(--success);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.item-actions {
		flex-shrink: 0;
	}

	.action-btn {
		padding: 6px 14px;
		font-size: 0.8rem;
		min-height: auto;
	}

	.processing {
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	.bottom-actions {
		text-align: center;
		margin-top: 24px;
		padding: 16px 0;
	}

	.done-btn {
		width: 100%;
		max-width: 320px;
		padding: 14px 24px;
		font-size: 1.1rem;
	}

	.done-hint {
		margin-top: 8px;
		font-size: 0.8rem;
		color: var(--text-muted);
	}
</style>
