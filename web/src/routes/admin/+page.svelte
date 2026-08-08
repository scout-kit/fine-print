<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { isAdmin } from '$lib/stores';
	import { adminLogin, getSetupStatus, listPhotos, listProjects, listQueue, type Photo, type Project, type QueueResponse } from '$lib/api';
	import PhotoThumb from '$lib/PhotoThumb.svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import { createSSE, type SSEConnection } from '$lib/sse';

	let authenticated = $state(false);
	isAdmin.subscribe(v => authenticated = v);

	// Login state
	let password = $state('');
	let loginError = $state('');
	let loggingIn = $state(false);

	// Dashboard state
	let photos: Photo[] = $state([]);
	let projects: Project[] = $state([]);
	let queue: QueueResponse | null = $state(null);
	let selectedPhoto: Photo | null = $state(null);
	let sseConn: SSEConnection | null = null;
	let loading = $state(false);

	async function handleLogin() {
		loggingIn = true;
		loginError = '';
		try {
			await adminLogin(password);
			isAdmin.set(true);
			authenticated = true;
			startDashboard();
		} catch (e) {
			const msg = e instanceof Error ? e.message : 'Login failed';
			if (msg === 'setup_required') {
				goto('/setup');
				return;
			}
			loginError = msg;
		}
		loggingIn = false;
	}

	async function refreshData() {
		if (loading) return;
		loading = true;
		try {
			[photos, queue, projects] = await Promise.all([
				listPhotos(),
				listQueue(),
				listProjects()
			]);
		} catch {
			// Ignore
		}
		loading = false;
	}

	function startDashboard() {
		refreshData();

		if (!sseConn) {
			sseConn = createSSE('/api/admin/events');
			let skipFirst = true;
			sseConn.state.subscribe(s => {
				// Skip the initial store value and the "connected" event
				if (skipFirst) {
					skipFirst = false;
					return;
				}
				if (s.lastEvent && s.lastEvent.type !== 'connected') {
					refreshData();
				}
			});
		}
	}

	onMount(async () => {
		// Bounce to the setup wizard if the kiosk hasn't been configured yet.
		try {
			const s = await getSetupStatus();
			if (s.needs_setup) {
				goto('/setup');
				return;
			}
		} catch {
			// Setup status is best-effort — if the server is unreachable we'll
			// fall through to the login form and surface the real error there.
		}
		if (authenticated) startDashboard();
	});

	onDestroy(() => {
		sseConn?.close();
		sseConn = null;
	});

	// Grouped counts
	let pendingCount = $derived(photos.filter(p => p.status_id === 1).length); // uploaded
	let inProgressCount = $derived(photos.filter(p => p.status_id >= 2 && p.status_id <= 4).length); // approved + queued + printing
	let printedCount = $derived(photos.filter(p => p.status_id === 5).length);
	let issuesCount = $derived(photos.filter(p => p.status_id === 6).length); // failed only

	function getProjectName(projectId: number | undefined): string {
		if (!projectId) return '';
		return projects.find(p => p.id === projectId)?.name || '';
	}
</script>

{#if !authenticated}
	<div class="container">
		<div class="login-box">
			<h1>Fine Print Admin</h1>
			<form onsubmit={(e) => { e.preventDefault(); handleLogin(); }}>
				<input
					type="password"
					placeholder="Admin password"
					bind:value={password}
					autocomplete="current-password"
				/>
				{#if loginError}
					<div class="alert error">{loginError}</div>
				{/if}
				<button class="primary" type="submit" disabled={loggingIn}>
					{loggingIn ? 'Logging in...' : 'Log In'}
				</button>
			</form>
			<a href="/" class="back-link">&larr; Back to upload</a>
		</div>
	</div>
{:else}
	<h2>Dashboard</h2>

	{#if queue?.paused}
		<div class="alert error">
			Print queue is paused. <a href="/admin/queue">View queue</a>
		</div>
	{/if}

	<div class="stats-grid">
		<a href="/admin/queue" class="stat-card">
			<span class="stat-value">{pendingCount}</span>
			<span class="stat-label">Pending Review</span>
		</a>
		<a href="/admin/queue#print" class="stat-card">
			<span class="stat-value">{inProgressCount}</span>
			<span class="stat-label">In Progress</span>
		</a>
		<a href="/admin/photos?status=printed" class="stat-card">
			<span class="stat-value">{printedCount}</span>
			<span class="stat-label">Printed</span>
		</a>
		<a href="/admin/photos?status=failed" class="stat-card">
			<span class="stat-value" class:has-issues={issuesCount > 0}>{issuesCount}</span>
			<span class="stat-label">Issues</span>
		</a>
	</div>

	{#if pendingCount > 0}
		<h3>Pending Review</h3>
		<div class="photo-grid">
			{#each photos.filter(p => p.status_id === 1) as photo (photo.id)}
				<PhotoThumb {photo} onclick={() => selectedPhoto = photo} />
			{/each}
		</div>
		<a href="/admin/photos" class="view-all">View all photos &rarr;</a>
	{/if}

	{#if selectedPhoto}
		<PhotoModal
			photo={selectedPhoto}
			onClose={() => selectedPhoto = null}
			onAction={refreshData}
			projectName={getProjectName(selectedPhoto?.project_id)}
		/>
	{/if}
{/if}

<style>
	.login-box {
		max-width: 360px;
		margin: 80px auto 0;
		text-align: center;
	}

	.login-box h1 {
		margin-bottom: 32px;
		font-size: 1.75rem;
	}

	.login-box form {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.back-link {
		display: block;
		margin-top: 24px;
		color: var(--text-muted);
		font-size: 0.875rem;
	}

	h2 {
		font-size: 1.5rem;
		margin-bottom: 16px;
	}

	h3 {
		font-size: 1.1rem;
		margin: 24px 0 12px;
	}

	.stats-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 12px;
		margin-bottom: 24px;
	}

	.stat-card {
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 16px;
		text-align: center;
	}

	.stat-value {
		display: block;
		font-size: 2rem;
		font-weight: 700;
		color: var(--accent);
	}

	.stat-value.has-issues {
		color: var(--danger);
	}

	.stat-label {
		font-size: 0.8rem;
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.photo-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
		gap: 8px;
	}

	.photo-thumb {
		aspect-ratio: 3/2;
		border-radius: var(--radius-sm);
		overflow: hidden;
		position: relative;
		background: var(--bg-elevated);
	}

	.photo-thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.photo-thumb .badge {
		position: absolute;
		bottom: 4px;
		left: 4px;
		font-size: 0.65rem;
	}

	.no-preview {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-muted);
		font-size: 0.75rem;
	}

	.view-all {
		display: block;
		text-align: center;
		margin-top: 12px;
		color: var(--accent);
		font-size: 0.875rem;
	}
</style>
