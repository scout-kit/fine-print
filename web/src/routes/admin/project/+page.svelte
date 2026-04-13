<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		listProjects, createProject, deleteProject, copyProject,
		VISIBILITY_PUBLIC, VISIBILITY_HIDDEN, VISIBILITY_PRIVATE, VISIBILITY_LABELS,
		PROJECT_TYPE_STANDARD, PROJECT_TYPE_BOOTH, PROJECT_TYPE_LABELS,
		type Project
	} from '$lib/api';

	let allProjects: Project[] = $state([]);
	let filterVisibility = $state(0); // 0 = all
	let newName = $state('');
	let newVisibility = $state(VISIBILITY_PUBLIC);

	let projects = $derived(
		filterVisibility === 0
			? allProjects
			: allProjects.filter(p => p.visibility_id === filterVisibility)
	);

	async function load() {
		allProjects = await listProjects();
	}

	onMount(load);

	async function handleCreate() {
		if (!newName.trim()) return;
		const project = await createProject(newName.trim(), newVisibility);
		newName = '';
		newVisibility = VISIBILITY_PUBLIC;
		goto(`/admin/project/${project.id}/settings`);
	}

	async function handleDelete(e: Event, id: number) {
		e.preventDefault();
		e.stopPropagation();
		if (!confirm('Delete this project?')) return;
		await deleteProject(id);
		load();
	}

	function projectUrl(project: Project): string {
		if (project.slug) {
			return `${window.location.origin}/p/${project.slug}`;
		}
		return `${window.location.origin}/?project=${project.id}`;
	}

	function copyLink(e: Event, project: Project) {
		e.preventDefault();
		e.stopPropagation();
		const url = projectUrl(project);
		if (navigator.clipboard?.writeText) {
			navigator.clipboard.writeText(url).catch(() => fallbackCopy(url));
		} else {
			fallbackCopy(url);
		}
	}

	function fallbackCopy(text: string) {
		const input = document.createElement('input');
		input.value = text;
		document.body.appendChild(input);
		input.select();
		document.execCommand('copy');
		document.body.removeChild(input);
	}

	async function handleCopy(e: Event, project: Project) {
		e.preventDefault();
		e.stopPropagation();
		const name = prompt('Name for the copy:', `${project.name} (copy)`);
		if (!name) return;
		const vis = prompt('Visibility (1=Public, 2=Hidden, 3=Private):', '1');
		const visId = Number(vis) || VISIBILITY_PUBLIC;
		const newProject = await copyProject(project.id, name, visId);
		goto(`/admin/project/${newProject.id}/settings`);
	}
</script>

<h2>Projects</h2>

<form class="create-form" onsubmit={(e) => { e.preventDefault(); handleCreate(); }}>
	<input type="text" placeholder="New project name" bind:value={newName} />
	<select bind:value={newVisibility} class="vis-select">
		<option value={VISIBILITY_PUBLIC}>Public</option>
		<option value={VISIBILITY_HIDDEN}>Hidden</option>
		<option value={VISIBILITY_PRIVATE}>Private</option>
	</select>
	<button class="primary" type="submit">Create</button>
</form>

<div class="list-header">
	<select bind:value={filterVisibility} class="filter-select">
		<option value={0}>All ({allProjects.length})</option>
		<option value={VISIBILITY_PUBLIC}>Public ({allProjects.filter(p => p.visibility_id === VISIBILITY_PUBLIC).length})</option>
		<option value={VISIBILITY_HIDDEN}>Hidden ({allProjects.filter(p => p.visibility_id === VISIBILITY_HIDDEN).length})</option>
		<option value={VISIBILITY_PRIVATE}>Private ({allProjects.filter(p => p.visibility_id === VISIBILITY_PRIVATE).length})</option>
	</select>
</div>

{#if projects.length === 0}
	<p class="empty">No projects yet. Create one to get started.</p>
{:else}
	<div class="project-list">
		{#each projects as project}
			<a href="/admin/project/{project.id}" class="project-card card">
				<div class="project-info">
					<div class="project-top">
						<h3>{project.name}</h3>
						<span class="vis-badge vis-{project.visibility_id}">{VISIBILITY_LABELS[project.visibility_id] || 'Public'}</span>
						{#if project.project_type_id === PROJECT_TYPE_BOOTH}
							<span class="vis-badge booth-badge">Booth</span>
						{/if}
					</div>
					<span class="project-date">{new Date(project.created_at).toLocaleDateString()}</span>
				</div>
				<div class="project-actions">
					{#if project.slug && project.visibility_id !== VISIBILITY_PRIVATE}
						<button class="link-btn" onclick={(e) => copyLink(e, project)} title="Copy link">
							<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
								<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
							</svg>
						</button>
					{/if}
					<button class="link-btn" onclick={(e) => handleCopy(e, project)} title="Copy project">
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
						</svg>
					</button>
					<button class="delete-btn" onclick={(e) => handleDelete(e, project.id)}>&times;</button>
				</div>
			</a>
		{/each}
	</div>
{/if}

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }

	.create-form {
		display: flex;
		gap: 8px;
		margin-bottom: 20px;
	}

	.create-form input { flex: 1; }
	.create-form button { white-space: nowrap; }

	.vis-select {
		width: auto;
		min-width: 100px;
		font-size: 0.85rem;
		padding: 8px 12px;
	}

	.list-header {
		margin-bottom: 12px;
	}

	.filter-select {
		font-size: 0.85rem;
		padding: 8px 12px;
		min-height: auto;
		width: auto;
	}

	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }

	.project-list { display: flex; flex-direction: column; gap: 8px; }

	.project-card {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 16px 20px;
		text-decoration: none;
		color: var(--text);
		transition: border-color 0.15s;
	}

	.project-card:hover { border-color: var(--accent); }

	.project-info { flex: 1; }

	.project-top {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 2px;
	}

	.project-top h3 { font-size: 1.05rem; font-weight: 600; }
	.project-date { font-size: 0.75rem; color: var(--text-muted); }

	.vis-badge {
		font-size: 0.65rem;
		padding: 2px 8px;
		border-radius: 999px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.vis-1 { background: #143d1f; color: var(--success); }
	.vis-2 { background: #3b2f00; color: var(--warning); }
	.vis-3 { background: #3b1a1a; color: var(--danger); }
	.booth-badge { background: #1a2a3b; color: #7ab8ff; }

	.project-actions { display: flex; gap: 4px; align-items: center; }

	.link-btn, .delete-btn {
		width: 32px; height: 32px; border-radius: 50%;
		background: transparent; color: var(--text-muted);
		display: flex; align-items: center; justify-content: center;
		padding: 0; min-height: auto; min-width: auto;
		transition: color 0.15s, background 0.15s;
	}

	.link-btn:hover { color: var(--accent); background: rgba(74, 158, 255, 0.1); }
	.delete-btn { font-size: 1.2rem; }
	.delete-btn:hover { color: var(--danger); background: rgba(248, 113, 113, 0.1); }
</style>
