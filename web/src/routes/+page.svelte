<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { uploadPhoto, listProjectsPublic, PROJECT_TYPE_BOOTH, type Project } from '$lib/api';

	let projects: Project[] = $state([]);
	let selectedProject: Project | null = $state(null);
	let uploading = $state(false);
	let uploadProgress = $state('');
	let error = $state('');
	let dragOver = $state(false);

	onMount(async () => {
		try {
			projects = await listProjectsPublic();
		} catch {
			// No projects
		}
	});

	function selectProject(p: Project) {
		// Booth projects redirect to the booth UI
		if (p.project_type_id === PROJECT_TYPE_BOOTH && p.slug) {
			goto(`/booth/${p.slug}`);
			return;
		}
		selectedProject = p;
	}

	function backToProjects() {
		selectedProject = null;
	}

	async function handleFiles(files: FileList | File[]) {
		if (!selectedProject) return;

		const imageFiles = Array.from(files).filter(f => f.type.startsWith('image/'));
		if (imageFiles.length === 0) {
			error = 'Please select image files';
			return;
		}

		uploading = true;
		error = '';

		const uploadedIds: number[] = [];

		for (let i = 0; i < imageFiles.length; i++) {
			uploadProgress = `Uploading ${i + 1} of ${imageFiles.length}...`;
			try {
				const result = await uploadPhoto(imageFiles[i], selectedProject.id);
				uploadedIds.push(result.id);
			} catch (e) {
				console.error('Upload failed for file:', imageFiles[i].name, e);
			}
		}

		uploading = false;
		uploadProgress = '';

		if (uploadedIds.length === 0) {
			error = 'All uploads failed';
			return;
		}

		if (uploadedIds.length === 1) {
			goto(`/edit?id=${uploadedIds[0]}`);
		} else {
			goto(`/review?ids=${uploadedIds.join(',')}`);
		}
	}

	function onFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (input.files?.length) {
			handleFiles(input.files);
		}
	}

	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		if (e.dataTransfer?.files?.length) {
			handleFiles(e.dataTransfer.files);
		}
	}

	function onDragOver(e: DragEvent) {
		e.preventDefault();
		dragOver = true;
	}

	function onDragLeave() {
		dragOver = false;
	}
</script>

<div class="container">
	<header>
		<h1>Fine Print</h1>
	</header>

	{#if projects.length === 0}
		<div class="card" style="text-align: center; margin-top: 24px;">
			<p style="color: var(--text-muted);">No projects yet. Ask the admin to create one.</p>
		</div>
	{:else if !selectedProject}
		<p class="pick-label">Choose a project</p>
		<div class="project-grid">
			{#each projects as project}
				<button class="project-card card" onclick={() => selectProject(project)}>
					<h3>{project.name}</h3>
				</button>
			{/each}
		</div>
	{:else}
		{#if projects.length > 1}
			<button class="back-btn" onclick={backToProjects}>&larr; {selectedProject.name}</button>
		{:else}
			<p class="event-name">{selectedProject.name}</p>
		{/if}

		<div
			class="upload-zone"
			class:drag-over={dragOver}
			class:uploading
			ondrop={onDrop}
			ondragover={onDragOver}
			ondragleave={onDragLeave}
			role="button"
			tabindex="0"
		>
			{#if uploading}
				<div class="spinner"></div>
				<p>{uploadProgress || 'Uploading...'}</p>
			{:else}
				<svg class="upload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
					<polyline points="17 8 12 3 7 8"/>
					<line x1="12" y1="3" x2="12" y2="15"/>
				</svg>
				<p class="upload-label">Upload photos</p>
				<p class="upload-hint">Select one or more, or drag and drop</p>
				<label class="upload-btn">
					Choose Photos
					<input
						type="file"
						accept="image/*"
						multiple
						onchange={onFileSelect}
						hidden
					/>
				</label>
			{/if}
		</div>
	{/if}

	{#if error}
		<div class="alert error">{error}</div>
	{/if}

	<nav class="bottom-nav">
		<a href="/gallery">Gallery</a>
		<span class="nav-sep">&middot;</span>
		<a href="/admin">Admin</a>
	</nav>
</div>

<style>
	header { text-align: center; padding: 32px 0 16px; }
	h1 { font-size: 2rem; font-weight: 700; letter-spacing: -0.02em; }
	.pick-label { text-align: center; color: var(--text-muted); margin-bottom: 16px; font-size: 0.9rem; }
	.project-grid { display: flex; flex-direction: column; gap: 10px; }
	.project-card { width: 100%; padding: 20px; text-align: center; cursor: pointer; transition: border-color 0.15s; font-family: inherit; color: var(--text); }
	.project-card:hover { border-color: var(--accent); }
	.project-card h3 { font-size: 1.15rem; font-weight: 600; }
	.back-btn { display: block; background: none; color: var(--accent); padding: 8px 0; font-size: 0.9rem; font-weight: 500; text-align: left; min-height: auto; }
	.event-name { text-align: center; color: var(--accent); font-size: 1.1rem; margin-top: 4px; }
	.upload-zone { margin-top: 16px; border: 2px dashed var(--border); border-radius: var(--radius); padding: 48px 24px; text-align: center; transition: border-color 0.2s, background 0.2s; display: flex; flex-direction: column; align-items: center; gap: 12px; }
	.upload-zone.drag-over { border-color: var(--accent); background: rgba(74, 158, 255, 0.05); }
	.upload-zone.uploading { pointer-events: none; opacity: 0.7; }
	.upload-icon { width: 48px; height: 48px; color: var(--text-muted); }
	.upload-label { font-size: 1.1rem; font-weight: 600; }
	.upload-hint { color: var(--text-muted); font-size: 0.875rem; }
	.upload-btn { display: inline-block; margin-top: 8px; padding: 12px 32px; background: var(--accent); color: white; border-radius: var(--radius-sm); font-weight: 600; font-size: 1rem; cursor: pointer; min-height: 44px; }
	.spinner { width: 40px; height: 40px; border: 3px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: spin 0.8s linear infinite; }
	@keyframes spin { to { transform: rotate(360deg); } }
	.bottom-nav { text-align: center; margin-top: 48px; padding: 16px 0; }
	.bottom-nav a { color: var(--text-muted); font-size: 0.875rem; }
	.nav-sep { color: var(--border); }
</style>
