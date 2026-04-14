<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { listPhotos, listProjects, type Photo, type Project } from '$lib/api';

	let photos: Photo[] = $state([]);
	let projects: Project[] = $state([]);
	let selectedPhoto: Photo | null = $state(null);
	let filterStatus = $state('');
	let filterProject = $state('');

	async function load() {
		const status = filterStatus || undefined;
		const projectId = filterProject ? Number(filterProject) : undefined;
		photos = await listPhotos(status, projectId);
	}

	onMount(async () => {
		// Read initial filters from URL query params
		filterStatus = page.url.searchParams.get('status') || '';
		filterProject = page.url.searchParams.get('project_id') || '';

		projects = await listProjects();
		load();
	});

	function projectName(id: number): string {
		return projects.find(p => p.id === id)?.name || `#${id}`;
	}
</script>

<h2>All Photos</h2>

<div class="filters">
	<select bind:value={filterStatus} onchange={() => load()}>
		<option value="">All statuses</option>
		<option value="uploaded">Uploaded</option>
		<option value="approved">Approved</option>
		<option value="queued">Queued</option>
		<option value="printing">Printing</option>
		<option value="printed">Printed</option>
		<option value="failed">Failed</option>
		<option value="rejected">Rejected</option>
	</select>
	<select bind:value={filterProject} onchange={() => load()}>
		<option value="">All projects</option>
		{#each projects as p}
			<option value={p.id}>{p.name}</option>
		{/each}
	</select>
</div>

{#if photos.length === 0}
	<p class="empty">No photos found.</p>
{:else}
	<p class="count">{photos.length} photo{photos.length !== 1 ? 's' : ''}</p>
	<div class="photo-grid">
		{#each photos as photo (photo.id)}
			<PhotoThumb {photo} onclick={() => selectedPhoto = photo} showProject={projectName(photo.project_id)} />
		{/each}
	</div>
{/if}

{#if selectedPhoto}
	<PhotoModal
		photo={selectedPhoto}
		onClose={() => selectedPhoto = null}
		onAction={load}
		projectName={projectName(selectedPhoto.project_id)}
	/>
{/if}

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }
	.filters { display: flex; gap: 8px; margin-bottom: 16px; }
	.filters select { flex: 1; font-size: 0.85rem; padding: 8px 12px; min-height: auto; }
	.count { font-size: 0.8rem; color: var(--text-muted); margin-bottom: 8px; }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.photo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
</style>
