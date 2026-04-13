<script lang="ts">
	import { page } from '$app/state';

	let { children } = $props();

	const projectId = $derived(page.params.id);
	const currentPath = $derived(page.url.pathname);

	const tabs = $derived([
		{ href: `/admin/project/${projectId}`, label: 'Photos', match: (p: string) => p === `/admin/project/${projectId}` },
		{ href: `/admin/project/${projectId}/template`, label: 'Template', match: (p: string) => p.includes('/template') },
		{ href: `/admin/project/${projectId}/settings`, label: 'Settings', match: (p: string) => p.includes('/settings') }
	]);
</script>

<a href="/admin/project" class="back">&larr; All Projects</a>

<div class="project-tabs">
	{#each tabs as tab}
		<a
			href={tab.href}
			class="tab"
			class:active={tab.match(currentPath)}
		>
			{tab.label}
		</a>
	{/each}
</div>

{@render children()}

<style>
	.back {
		display: inline-block;
		color: var(--text-muted);
		font-size: 0.875rem;
		margin-bottom: 12px;
	}

	.project-tabs {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border);
		margin-bottom: 20px;
	}

	.tab {
		padding: 10px 20px;
		color: var(--text-muted);
		font-weight: 500;
		border-bottom: 2px solid transparent;
		font-size: 0.875rem;
		text-decoration: none;
		min-height: 44px;
		display: flex;
		align-items: center;
	}

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}
</style>
