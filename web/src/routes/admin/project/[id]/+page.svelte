<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { getProject, listPhotos, uploadPhoto, type Photo } from '$lib/api';

	const projectId = $derived(Number(page.params.id));
	let projectName = $state('');
	let photos: Photo[] = $state([]);
	let uploading = $state(false);
	let selectedPhoto: Photo | null = $state(null);

	async function load() {
		try {
			const data = await getProject(projectId);
			projectName = data.project.name;
		} catch { /* ignore */ }
		try {
			photos = await listPhotos(undefined, projectId);
		} catch { photos = []; }
	}

	onMount(load);

	async function handlePhotoUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.length) return;
		uploading = true;
		for (const file of Array.from(input.files)) {
			try { await uploadPhoto(file, projectId); } catch { /* ignore */ }
		}
		input.value = '';
		uploading = false;
		await load();
		pollForPreviews();
	}

	function pollForPreviews() {
		if (!photos.some(p => !p.preview_key)) return;
		const interval = setInterval(async () => {
			await load();
			if (!photos.some(p => !p.preview_key)) clearInterval(interval);
		}, 2000);
		setTimeout(() => clearInterval(interval), 30000);
	}
</script>

<h2>{projectName || 'Photos'}</h2>

<div class="toolbar">
	<label class="upload-btn primary" class:uploading>
		{uploading ? 'Uploading...' : 'Upload Photos'}
		<input type="file" accept="image/*" multiple hidden onchange={handlePhotoUpload} disabled={uploading} />
	</label>
</div>

{#if photos.length === 0}
	<p class="empty">No photos uploaded to this project yet.</p>
{:else}
	<div class="photo-grid">
		{#each photos as photo (photo.id)}
			<PhotoThumb {photo} onclick={() => selectedPhoto = photo} />
		{/each}
	</div>
{/if}

{#if selectedPhoto}
	<PhotoModal
		photo={selectedPhoto}
		onClose={() => selectedPhoto = null}
		onAction={load}
		projectName={projectName}
	/>
{/if}

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }
	.toolbar { margin-bottom: 16px; }
	.upload-btn { display: inline-block; padding: 10px 20px; background: var(--accent); color: white; border-radius: var(--radius-sm); font-weight: 600; font-size: 0.875rem; cursor: pointer; min-height: 44px; }
	.upload-btn.uploading { opacity: 0.6; pointer-events: none; }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.photo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
</style>
