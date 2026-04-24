<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { adminSession, adminLogout, getStorage, type DiskUsage } from '$lib/api';
	import { isAdmin } from '$lib/stores';
	import { createSSE, type SSEConnection } from '$lib/sse';

	let { children } = $props();
	let checking = $state(true);
	let authenticated = $state(false);
	let storageUsage: DiskUsage | null = $state(null);
	let sseConn: SSEConnection | null = null;

	// React to the isAdmin store so login from child pages updates the layout
	isAdmin.subscribe(v => authenticated = v);

	async function refreshStorage() {
		try {
			const s = await getStorage();
			storageUsage = s.enabled ? (s.usage ?? null) : null;
		} catch { /* ignore */ }
	}

	onMount(async () => {
		try {
			await adminSession();
			isAdmin.set(true);
			await refreshStorage();
			// Poll once a minute — not worth wiring SSE for this.
			setInterval(refreshStorage, 60_000);
			// Surface printer disconnect events via the alert slot we already have.
			sseConn = createSSE('/api/admin/events');
		} catch {
			isAdmin.set(false);
		}
		checking = false;
	});

	onDestroy(() => sseConn?.close());

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
		{#if storageUsage && (storageUsage.warn_active || !storageUsage.above_min_free)}
			<div class="storage-banner" class:critical={!storageUsage.above_min_free}>
				{storageUsage.message || 'Disk is filling up.'}
			</div>
		{/if}
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

	.storage-banner {
		padding: 10px 16px;
		font-size: 0.85rem;
		text-align: center;
		background: var(--warning-bg, rgba(200, 140, 0, 0.15));
		color: var(--warning, #c88c00);
		border-bottom: 1px solid var(--border);
	}

	.storage-banner.critical {
		background: var(--danger-bg, rgba(200, 60, 60, 0.15));
		color: var(--danger, #c83c3c);
		font-weight: 500;
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
