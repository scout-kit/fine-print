<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import CameraBooth from '$lib/CameraBooth.svelte';
	import { getProjectBySlug, PROJECT_TYPE_BOOTH, type ProjectResponse } from '$lib/api';

	const slug = $derived(page.params.slug);
	let project: ProjectResponse | null = $state(null);
	let error = $state('');

	onMount(async () => {
		try {
			project = await getProjectBySlug(slug);
			if (project.project.project_type_id !== PROJECT_TYPE_BOOTH) {
				// Not a booth project, redirect to normal upload
				goto(`/p/${slug}`);
				return;
			}
		} catch {
			error = 'Project not found';
		}
	});
</script>

{#if error}
	<div class="booth-error">
		<h2>Photo Booth</h2>
		<p>{error}</p>
		<a href="/">Back to home</a>
	</div>
{:else if !project}
	<div class="booth-loading">
		<div class="spinner"></div>
	</div>
{:else}
	<CameraBooth
		projectId={project.project.id}
		projectName={project.project.name}
	/>
{/if}

<style>
	.booth-error {
		min-height: 100vh;
		background: #000;
		color: #fff;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
	}

	.booth-error a {
		color: var(--accent);
	}

	.booth-loading {
		min-height: 100vh;
		background: #000;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.spinner {
		width: 48px;
		height: 48px;
		border: 3px solid #333;
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin { to { transform: rotate(360deg); } }
</style>
