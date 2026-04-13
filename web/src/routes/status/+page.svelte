<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { photoStatus, photoStatusName, downloadOriginalUrl, downloadRenderedUrl } from '$lib/api';
	import { createSSE, type SSEConnection } from '$lib/sse';

	const photoId = $derived(Number(page.url.searchParams.get('id')));
	let status = $state('');
	let statusId = $state(0);
	let sse: SSEConnection | null = null;

	async function loadStatus() {
		if (!photoId) return;
		try {
			const result = await photoStatus(photoId);
			status = result.status;
			statusId = getStatusId(result.status);
		} catch {
			// Ignore
		}
	}

	onMount(() => {
		loadStatus();
		sse = createSSE('/api/events');
		let skipFirst = true;
		sse.state.subscribe(s => {
			if (skipFirst) { skipFirst = false; return; }
			if (s.lastEvent?.type === 'photo_status' || s.lastEvent?.type === 'print_status') {
				const data = s.lastEvent.data as { photo_id?: number; status?: string };
				if (data.photo_id === photoId) {
					status = data.status || '';
					statusId = getStatusId(status);
				}
			}
		});
	});

	function getStatusId(name: string): number {
		const map: Record<string, number> = {
			uploaded: 1, approved: 2, queued: 3,
			printing: 4, printed: 5, failed: 6, rejected: 7
		};
		return map[name] || 0;
	}

	const steps = [
		{ id: 1, label: 'Uploaded', icon: 'cloud-upload' },
		{ id: 2, label: 'Approved', icon: 'check-circle' },
		{ id: 3, label: 'Queued', icon: 'clock' },
		{ id: 4, label: 'Printing', icon: 'printer' },
		{ id: 5, label: 'Printed', icon: 'check-double' }
	];

	onDestroy(() => sse?.close());
</script>

<div class="container">
	<header>
		<a href="/" class="back">&larr; Upload another</a>
		<h2>Photo Status</h2>
	</header>

	{#if !photoId}
		<div class="card" style="text-align: center;">
			<p>No photo selected</p>
		</div>
	{:else if statusId === 6}
		<div class="alert error">
			Something went wrong with your photo. Please try uploading again.
		</div>
	{:else if statusId === 7}
		<div class="alert warning">
			Your photo was not approved.
		</div>
	{:else}
		<div class="progress-tracker">
			{#each steps as step, i}
				{@const isActive = statusId >= step.id}
				{@const isCurrent = statusId === step.id}
				<div class="step" class:active={isActive} class:current={isCurrent}>
					<div class="step-dot">
						{#if isActive && !isCurrent}
							<svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16">
								<path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>
							</svg>
						{:else if isCurrent}
							<div class="pulse"></div>
						{:else}
							<span>{i + 1}</span>
						{/if}
					</div>
					<span class="step-label">{step.label}</span>
				</div>
				{#if i < steps.length - 1}
					<div class="step-line" class:active={statusId > step.id}></div>
				{/if}
			{/each}
		</div>

		{#if statusId === 5}
			<div class="done-message">
				<p>Your photo has been printed!</p>
				<div class="download-btns">
					<a href={downloadOriginalUrl(photoId)} class="dl-btn" download>Download Original</a>
					<a href={downloadRenderedUrl(photoId)} class="dl-btn primary" download>Download Print Version</a>
				</div>
				<a href="/" class="upload-another">Upload another photo</a>
			</div>
		{/if}
	{/if}
</div>

<style>
	header {
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 16px 0;
	}

	.back {
		color: var(--text-muted);
		font-size: 0.875rem;
	}

	h2 {
		font-size: 1.25rem;
		font-weight: 600;
	}

	.progress-tracker {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0;
		padding: 48px 0;
		flex-wrap: wrap;
	}

	.step {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		min-width: 60px;
	}

	.step-dot {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background: var(--bg-elevated);
		border: 2px solid var(--border);
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-muted);
		font-size: 0.75rem;
		font-weight: 600;
		transition: all 0.3s;
	}

	.step.active .step-dot {
		background: var(--accent);
		border-color: var(--accent);
		color: white;
	}

	.step.current .step-dot {
		background: var(--accent);
		border-color: var(--accent);
		color: white;
		box-shadow: 0 0 0 4px rgba(74, 158, 255, 0.3);
	}

	.step-label {
		font-size: 0.75rem;
		color: var(--text-muted);
		text-align: center;
	}

	.step.active .step-label {
		color: var(--text);
		font-weight: 600;
	}

	.step-line {
		width: 24px;
		height: 2px;
		background: var(--border);
		margin-bottom: 24px;
		transition: background 0.3s;
	}

	.step-line.active {
		background: var(--accent);
	}

	.pulse {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: white;
		animation: pulse-anim 1.5s ease-in-out infinite;
	}

	@keyframes pulse-anim {
		0%, 100% { opacity: 1; transform: scale(1); }
		50% { opacity: 0.6; transform: scale(0.8); }
	}

	.done-message {
		text-align: center;
		padding: 24px;
	}

	.done-message p {
		font-size: 1.25rem;
		font-weight: 600;
		color: var(--success);
		margin-bottom: 16px;
	}

	.download-btns {
		display: flex;
		gap: 8px;
		justify-content: center;
		margin-bottom: 16px;
		flex-wrap: wrap;
	}

	.dl-btn {
		display: inline-block;
		padding: 10px 20px;
		border-radius: var(--radius-sm);
		font-weight: 600;
		font-size: 0.875rem;
		text-decoration: none;
		border: 1px solid var(--border);
		color: var(--text-muted);
	}

	.dl-btn.primary {
		background: var(--accent);
		color: white;
		border-color: var(--accent);
	}

	.upload-another {
		display: inline-block;
		padding: 12px 32px;
		background: var(--accent);
		color: white;
		border-radius: var(--radius-sm);
		font-weight: 600;
	}
</style>
