<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { listPhotos, listProjects, exportPhotos, exportProjectUrl, type Photo, type Project } from '$lib/api';

	let photos: Photo[] = $state([]);
	let projects: Project[] = $state([]);
	let selectedPhoto: Photo | null = $state(null);
	let filterStatus = $state('');
	let filterProject = $state('');

	let selectMode = $state(false);
	let selectedIds = $state<Set<number>>(new Set());
	let exporting = $state(false);

	async function load() {
		const status = filterStatus || undefined;
		const projectId = filterProject ? Number(filterProject) : undefined;
		photos = await listPhotos(status, projectId);
	}

	onMount(async () => {
		filterStatus = page.url.searchParams.get('status') || '';
		filterProject = page.url.searchParams.get('project_id') || '';
		projects = await listProjects();
		load();
	});

	function projectName(id: number): string {
		return projects.find(p => p.id === id)?.name || `#${id}`;
	}

	function toggleSelect(id: number) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id); else next.add(id);
		selectedIds = next;
	}

	function selectAll() { selectedIds = new Set(photos.map(p => p.id)); }
	function deselectAll() { selectedIds = new Set(); }

	function exitSelectMode() {
		selectMode = false;
		selectedIds = new Set();
	}

	async function handleExportSelected() {
		if (selectedIds.size === 0) return;
		exporting = true;
		try {
			await exportPhotos([...selectedIds]);
		} catch { /* ignore */ }
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

<div class="toolbar">
	{#if selectMode}
		<button class="tool-btn" onclick={selectAll}>Select All</button>
		<button class="tool-btn" onclick={deselectAll}>Deselect</button>
		<span class="select-count">{selectedIds.size} selected</span>
		<button class="tool-btn primary" onclick={handleExportSelected} disabled={selectedIds.size === 0 || exporting}>
			{exporting ? 'Exporting...' : 'Download ZIP'}
		</button>
		<button class="tool-btn" onclick={exitSelectMode}>Cancel</button>
	{:else}
		<button class="tool-btn" onclick={() => selectMode = true}>Select</button>
		{#if filterProject}
			<a href={exportProjectUrl(Number(filterProject))} class="tool-btn" download>Download All (ZIP)</a>
		{/if}
	{/if}
</div>

{#if photos.length === 0}
	<p class="empty">No photos found.</p>
{:else}
	<p class="count">{photos.length} photo{photos.length !== 1 ? 's' : ''}</p>
	<div class="photo-grid">
		{#each photos as photo (photo.id)}
			<PhotoThumb
				{photo}
				onclick={() => handleThumbClick(photo)}
				showProject={projectName(photo.project_id)}
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
		projectName={projectName(selectedPhoto.project_id)}
	/>
{/if}

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }
	.filters { display: flex; gap: 8px; margin-bottom: 12px; }
	.filters select { flex: 1; font-size: 0.85rem; padding: 8px 12px; min-height: auto; }

	.toolbar { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
	.tool-btn {
		padding: 6px 14px; font-size: 0.8rem; font-weight: 500;
		border: 1px solid var(--border); border-radius: var(--radius-sm);
		background: transparent; color: var(--text-muted); cursor: pointer;
		text-decoration: none; min-height: auto; min-width: auto;
		display: inline-flex; align-items: center;
	}
	.tool-btn:hover { border-color: var(--accent); color: var(--text); }
	.tool-btn.primary { background: var(--accent); color: white; border-color: var(--accent); }
	.tool-btn.primary:hover { opacity: 0.85; }
	.tool-btn:disabled { opacity: 0.5; pointer-events: none; }
	.select-count { font-size: 0.8rem; color: var(--text-muted); }

	.count { font-size: 0.8rem; color: var(--text-muted); margin-bottom: 8px; }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.photo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
</style>
