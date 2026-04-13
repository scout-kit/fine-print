<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { adminSession, adminLogout } from '$lib/api';
	import { isAdmin } from '$lib/stores';

	let { children } = $props();
	let checking = $state(true);
	let authenticated = $state(false);

	// React to the isAdmin store so login from child pages updates the layout
	isAdmin.subscribe(v => authenticated = v);

	onMount(async () => {
		try {
			await adminSession();
			isAdmin.set(true);
		} catch {
			isAdmin.set(false);
		}
		checking = false;
	});

	async function handleLogout() {
		await adminLogout();
		authenticated = false;
		isAdmin.set(false);
		goto('/');
	}

	const navItems = [
		{ href: '/admin', label: 'Dashboard' },
		{ href: '/admin/photos', label: 'Photos' },
		{ href: '/admin/queue', label: 'Queue' },
		{ href: '/admin/project', label: 'Projects' },
		{ href: '/admin/settings', label: 'Settings' }
	];
</script>

{#if checking}
	<div class="container" style="text-align: center; padding-top: 48px;">
		<div class="spinner"></div>
	</div>
{:else if !authenticated}
	{@render children()}
{:else}
	<div class="admin-shell">
		<nav class="admin-nav">
			{#each navItems as item}
				<a
					href={item.href}
					class="nav-item"
					class:active={page.url.pathname === item.href}
				>
					{item.label}
				</a>
			{/each}
			<button class="nav-item logout" onclick={handleLogout}>Logout</button>
		</nav>
		<main>
			{@render children()}
		</main>
	</div>
{/if}

<style>
	.admin-shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.admin-nav {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border);
		background: var(--bg-surface);
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
	}

	.nav-item {
		padding: 12px 16px;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--text-muted);
		text-decoration: none;
		white-space: nowrap;
		border-bottom: 2px solid transparent;
		background: none;
		font-family: inherit;
		cursor: pointer;
		min-height: 44px;
		display: flex;
		align-items: center;
	}

	.nav-item.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.nav-item.logout {
		margin-left: auto;
		color: var(--danger);
		border: none;
	}

	main {
		flex: 1;
		padding: 16px;
		max-width: 720px;
		margin: 0 auto;
		width: 100%;
	}

	.spinner {
		width: 40px;
		height: 40px;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
		margin: 0 auto;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}
</style>
