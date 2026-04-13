<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { getGallery, type GalleryPhoto } from '$lib/api';
	import { createSSE, type SSEConnection } from '$lib/sse';

	let photos: GalleryPhoto[] = $state([]);
	let selectedPhoto: GalleryPhoto | null = $state(null);
	let guestSession = $state('');
	let loading = $state(false);
	let sse: SSEConnection | null = null;

	async function load() {
		if (loading) return;
		loading = true;
		try { photos = await getGallery(); } catch { /* ignore */ }
		loading = false;
	}

	onMount(() => {
		load();
		const match = document.cookie.match(/fineprint_guest=([^;]+)/);
		if (match) guestSession = match[1];

		sse = createSSE('/api/events');
		let skipFirst = true;
		sse.state.subscribe(s => {
			if (skipFirst) { skipFirst = false; return; }
			if (s.lastEvent && s.lastEvent.type !== 'connected') load();
		});
	});

	onDestroy(() => sse?.close());
</script>

<div class="container">
	<header>
		<a href="/" class="back">&larr; Upload</a>
		<h2>Gallery</h2>
	</header>

	{#if photos.length === 0}
		<p class="empty">No photos yet. Be the first to upload!</p>
	{:else}
		<div class="gallery-grid">
			{#each photos as photo (photo.id)}
				<PhotoThumb {photo} onclick={() => selectedPhoto = photo} />
			{/each}
		</div>
	{/if}
</div>

{#if selectedPhoto}
	<PhotoModal
		photo={selectedPhoto}
		{guestSession}
		onClose={() => selectedPhoto = null}
		onAction={load}
	/>
{/if}

<style>
	header { display: flex; align-items: center; gap: 16px; padding: 16px 0; }
	.back { color: var(--text-muted); font-size: 0.875rem; }
	h2 { font-size: 1.25rem; font-weight: 600; }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.gallery-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 6px; }
</style>
