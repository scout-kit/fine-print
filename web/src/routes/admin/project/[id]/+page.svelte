<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { getProject, listPhotos, uploadPhoto, exportPhotos, exportProjectUrl, type Photo } from '$lib/api';

	const projectId = $derived(Number(page.params.id));
	let projectName = $state('');
	let photos: Photo[] = $state([]);
	let uploading = $state(false);
	let selectedPhoto: Photo | null = $state(null);

	let selectMode = $state(false);
	let selectedIds = $state<Set<number>>(new Set());
	let exporting = $state(false);

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

	function toggleSelect(id: number) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id); else next.add(id);
		selectedIds = next;
		if (next.size === 0) selectMode = false;
	}

	function enterSelectMode(id: number) {
		selectMode = true;
		selectedIds = new Set([id]);
	}

	function selectAll() { selectedIds = new Set(photos.map(p => p.id)); }

	function exitSelectMode() { selectMode = false; selectedIds = new Set(); }

	async function handleExportSelected() {
		if (selectedIds.size === 0) return;
		exporting = true;
		try { await exportPhotos([...selectedIds]); } catch { /* ignore */ }
		exporting = false;
	}

	function handleThumbClick(photo: Photo) {
		if (selectMode) {
			toggleSelect(photo.id);
		} else {
			selectedPhoto = photo;
		}
	}
</script>

<h2>{projectName || 'Photos'}</h2>

<div class="toolbar">
	<label class="upload-btn primary" class:uploading>
		{uploading ? 'Uploading...' : 'Upload Photos'}
		<input type="file" accept="image/*" multiple hidden onchange={handlePhotoUpload} disabled={uploading} />
	</label>

	{#if selectMode}
		<button class="tool-btn" onclick={selectAll}>Select All</button>
		<span class="select-count">{selectedIds.size} selected</span>
		<button class="tool-btn accent" onclick={handleExportSelected} disabled={selectedIds.size === 0 || exporting}>
			{exporting ? 'Exporting...' : 'Download ZIP'}
		</button>
		<button class="tool-btn" onclick={exitSelectMode}>Cancel</button>
	{:else if photos.length > 0}
		<a href={exportProjectUrl(projectId)} class="tool-btn" download>Download All</a>
	{/if}
</div>

{#if photos.length === 0}
	<p class="empty">No photos uploaded to this project yet.</p>
{:else}
	<div class="photo-grid">
		{#each photos as photo (photo.id)}
			<PhotoThumb
				{photo}
				onclick={() => handleThumbClick(photo)}
				onlongpress={() => enterSelectMode(photo.id)}
				selectable={selectMode}
				selected={selectedIds.has(photo.id)}
			/>
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

	.toolbar { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; flex-wrap: wrap; }
	.upload-btn {
		display: inline-block; padding: 8px 16px;
		background: var(--accent); color: white;
		border-radius: var(--radius-sm); font-weight: 600;
		font-size: 0.8rem; cursor: pointer; min-height: auto;
	}
	.upload-btn.uploading { opacity: 0.6; pointer-events: none; }

	.tool-btn {
		padding: 6px 14px; font-size: 0.8rem; font-weight: 500;
		border: 1px solid var(--border); border-radius: var(--radius-sm);
		background: transparent; color: var(--text-muted); cursor: pointer;
		text-decoration: none; min-height: auto; min-width: auto;
		display: inline-flex; align-items: center;
	}
	.tool-btn:hover { border-color: var(--accent); color: var(--text); }
	.tool-btn.accent { background: var(--accent); color: white; border-color: var(--accent); }
	.tool-btn.accent:hover { opacity: 0.85; }
	.tool-btn:disabled { opacity: 0.5; pointer-events: none; }
	.select-count { font-size: 0.8rem; color: var(--text-muted); }

	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.photo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
</style>
