<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import PhotoModal from '$lib/PhotoModal.svelte';
	import {
		listPhotos, listProjects, listQueue, approvePhoto, rejectPhoto,
		pauseQueue, resumeQueue, retryJob, cancelJob,
		previewUrl, renderPreviewUrl, photoStatusName, jobStatusName,
		type Photo, type Project, type QueueResponse, type PrintJob
	} from '$lib/api';
	import { createSSE, type SSEConnection } from '$lib/sse';

	let pendingPhotos: Photo[] = $state([]); // uploaded — for review tab
	let allPhotos: Photo[] = $state([]);    // all — for queue job matching
	let projects: Project[] = $state([]);
	let queue: QueueResponse | null = $state(null);
	let activeTab = $state<'review' | 'queue'>('review');
	let loading = $state(false);
	let sse: SSEConnection | null = null;
	let selectedPhoto: Photo | null = $state(null);

	// Per-photo copies for inline editing on approval
	let copiesMap = $state<Record<number, number>>({});
	let cacheBust = $state(Date.now());

	async function load() {
		if (loading) return;
		loading = true;
		try {
			[pendingPhotos, allPhotos, queue, projects] = await Promise.all([
				listPhotos('uploaded'),
				listPhotos(),
				listQueue(),
				listProjects()
			]);
			// Initialize copies from photo data
			for (const p of pendingPhotos) {
				if (!(p.id in copiesMap)) {
					copiesMap[p.id] = p.copies || 1;
				}
			}
		} catch { /* ignore */ }
		cacheBust = Date.now();
		loading = false;
	}

	onMount(() => {
		// Check hash for initial tab
		if (window.location.hash === '#print') {
			activeTab = 'queue';
		}
		load();
		sse = createSSE('/api/admin/events');
		let skipFirst = true;
		sse.state.subscribe(s => {
			if (skipFirst) { skipFirst = false; return; }
			if (s.lastEvent && s.lastEvent.type !== 'connected') load();
		});
	});

	onDestroy(() => sse?.close());

	// pendingPhotos is loaded directly with status=uploaded filter

	function getPhotoForJob(job: PrintJob): Photo | undefined {
		return allPhotos.find(p => p.id === job.photo_id);
	}

	function getProjectName(projectId: number | undefined): string {
		if (!projectId) return '';
		return projects.find(p => p.id === projectId)?.name || '';
	}

	async function handleApprove(id: number) {
		// Save copies first if changed
		const c = copiesMap[id];
		if (c && c > 0) {
			const { saveEdits, getEdits } = await import('$lib/api');
			try {
				const edits = await getEdits(id);
				await saveEdits(id, {
					crop_x: edits?.transform?.crop_x ?? 0,
					crop_y: edits?.transform?.crop_y ?? 0,
					crop_width: edits?.transform?.crop_width ?? 1,
					crop_height: edits?.transform?.crop_height ?? 1,
					rotation: edits?.transform?.rotation ?? 0,
					brightness: edits?.overrides?.brightness ?? null,
					contrast: edits?.overrides?.contrast ?? null,
					saturation: edits?.overrides?.saturation ?? null,
					overlay_overrides: [],
					text_overrides: [],
					copies: c
				});
			} catch { /* ignore */ }
		}
		await approvePhoto(id);
		load();
	}

	async function handleReject(id: number) { await rejectPhoto(id); load(); }
	async function handlePause() { await pauseQueue(); load(); }
	async function handleResume() { await resumeQueue(); load(); }
	async function handleRetry(id: number) { await retryJob(id); load(); }
	async function handleCancel(id: number) { await cancelJob(id); load(); }
</script>

<div class="tabs">
	<button class="tab" class:active={activeTab === 'review'} onclick={() => activeTab = 'review'}>
		Review ({pendingPhotos.length})
	</button>
	<button class="tab" class:active={activeTab === 'queue'} onclick={() => activeTab = 'queue'}>
		Print Queue ({queue?.jobs.length || 0})
	</button>
</div>

{#if queue?.paused}
	<div class="alert error">
		Queue paused
		<button class="success" style="margin-left: 12px; padding: 6px 16px;" onclick={handleResume}>Resume</button>
	</div>
{/if}

{#if activeTab === 'review'}
	<h2>Photos Pending Review</h2>

	{#if pendingPhotos.length === 0}
		<p class="empty">No photos awaiting review</p>
	{:else}
		<div class="review-list">
			{#each pendingPhotos as photo (photo.id)}
				<div class="review-row card">
					<button class="row-thumb" onclick={() => selectedPhoto = photo}>
						{#if photo.preview_key}
							<img src={renderPreviewUrl(photo.id) + '?t=' + cacheBust} alt="Photo {photo.id}" />
						{:else}
							<div class="no-preview">Processing</div>
						{/if}
					</button>
					<div class="review-info">
						{#if getProjectName(photo.project_id)}
							<span class="project-tag">{getProjectName(photo.project_id)}</span>
						{/if}
						<label class="copies-inline">
							<span>Copies</span>
							<input type="number" min="1" max="99"
								value={copiesMap[photo.id] || 1}
								oninput={(e) => copiesMap[photo.id] = Number((e.target as HTMLInputElement).value)}
							/>
						</label>
					</div>
					<div class="review-btns">
						<button class="success sm" onclick={() => handleApprove(photo.id)}>Approve</button>
						<button class="danger sm" onclick={() => handleReject(photo.id)}>Reject</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

{:else}
	<div class="queue-header">
		<h2>Print Queue</h2>
		<div class="queue-controls">
			{#if queue?.paused}
				<button class="success" onclick={handleResume}>Resume</button>
			{:else}
				<button class="ghost" onclick={handlePause}>Pause</button>
			{/if}
		</div>
	</div>

	{#if !queue?.jobs.length}
		<p class="empty">Queue is empty</p>
	{:else}
		<div class="job-list">
			{#each queue.jobs as job (job.id)}
				{@const photo = getPhotoForJob(job)}
				<div class="job-row card">
					<button class="job-thumb" onclick={() => { if (photo) selectedPhoto = photo; }}>
						{#if photo}
							<img src={renderPreviewUrl(photo.id) + '?t=' + cacheBust} alt="Job {job.id}" />
						{:else}
							<div class="no-preview">#{job.photo_id}</div>
						{/if}
					</button>
					<div class="job-info">
						<span class="badge {jobStatusName(job.status_id)}">{jobStatusName(job.status_id)}</span>
						{#if getProjectName(photo?.project_id)}
							<span class="project-tag">{getProjectName(photo?.project_id)}</span>
						{/if}
						{#if job.printer_name}
							<span class="job-meta">{job.printer_name}</span>
						{/if}
						{#if job.error_msg}
							<span class="job-error">{job.error_msg}</span>
						{/if}
					</div>
					<div class="job-btns">
						{#if job.status_id === 4}
							<button class="ghost sm" onclick={() => handleRetry(job.id)}>Retry</button>
						{/if}
						{#if job.status_id === 1 || job.status_id === 4}
							<button class="ghost sm danger-text" onclick={() => handleCancel(job.id)}>Cancel</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
{/if}

{#if selectedPhoto}
	<PhotoModal
		photo={selectedPhoto}
		onClose={() => selectedPhoto = null}
		onAction={load}
		projectName={getProjectName(selectedPhoto?.project_id)}
	/>
{/if}

<style>
	.tabs { display: flex; gap: 0; border-bottom: 1px solid var(--border); margin-bottom: 16px; }
	.tab { padding: 10px 20px; background: none; color: var(--text-muted); font-weight: 500; border-bottom: 2px solid transparent; border-radius: 0; min-height: 44px; font-size: 0.875rem; }
	.tab.active { color: var(--accent); border-bottom-color: var(--accent); }

	h2 { font-size: 1.25rem; margin-bottom: 12px; }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }

	/* Review list */
	.review-list { display: flex; flex-direction: column; gap: 6px; }

	.review-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 12px;
	}

	.row-thumb {
		width: 80px; height: 54px;
		border-radius: 6px; overflow: hidden;
		flex-shrink: 0; background: #000;
		border: none; padding: 0; cursor: pointer;
		min-height: auto; min-width: auto;
	}

	.row-thumb img { width: 100%; height: 100%; object-fit: contain; }

	.no-preview {
		display: flex; align-items: center; justify-content: center;
		height: 100%; color: var(--text-muted); font-size: 0.7rem;
	}

	.review-info { flex: 1; display: flex; align-items: center; gap: 8px; }
	.review-id { font-size: 0.8rem; color: var(--text-muted); font-weight: 600; }

	.copies-inline { display: flex; align-items: center; gap: 6px; }
	.copies-inline span { font-size: 0.75rem; color: var(--text-muted); }
	.copies-inline input { width: 50px; padding: 4px 6px; text-align: center; font-size: 0.8rem; min-height: auto; }
	.review-btns { display: flex; gap: 4px; flex-shrink: 0; }

	/* Queue table */
	.job-list { display: flex; flex-direction: column; gap: 6px; }

	.job-row {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 8px 12px;
	}

	.job-thumb {
		width: 80px; height: 54px;
		border-radius: 6px; overflow: hidden;
		flex-shrink: 0; background: var(--bg-elevated);
		border: none; padding: 0; cursor: pointer;
		min-height: auto; min-width: auto;
	}

	.job-thumb img { width: 100%; height: 100%; object-fit: contain; background: #000; }

	.job-info { flex: 1; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.job-meta { font-size: 0.75rem; color: var(--text-muted); }
	.job-error { font-size: 0.75rem; color: var(--danger); width: 100%; }
	.job-btns { display: flex; gap: 4px; flex-shrink: 0; }

	.project-tag { font-size: 0.7rem; color: var(--accent); font-weight: 500; }

	.sm { padding: 4px 10px; font-size: 0.75rem; min-height: auto; min-width: auto; }

	.job-error {
		display: block;
		font-size: 0.7rem;
		color: var(--danger);
		margin-bottom: 4px;
	}

	.danger-text { color: var(--danger); }

	.queue-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
	.queue-header h2 { margin: 0; }
	.queue-controls button { padding: 8px 16px; font-size: 0.875rem; }
</style>
