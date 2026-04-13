<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		getProject, updateProject, deleteProject,
		VISIBILITY_PUBLIC, VISIBILITY_HIDDEN, VISIBILITY_PRIVATE,
		PROJECT_TYPE_STANDARD, PROJECT_TYPE_BOOTH,
		type ProjectResponse
	} from '$lib/api';
	import { goto } from '$app/navigation';

	const projectId = $derived(Number(page.params.id));
	let data: ProjectResponse | null = $state(null);
	let saving = $state(false);
	let saved = $state(false);

	let editName = $state('');
	let editBrightness = $state(0);
	let editContrast = $state(0);
	let editSaturation = $state(0);
	let editVisibility = $state(VISIBILITY_PUBLIC);
	let editProjectType = $state(PROJECT_TYPE_STANDARD);
	let editBoothCountdown = $state(0);
	let slug = $state('');

	async function load() {
		try {
			data = await getProject(projectId);
			editName = data.project.name;
			editBrightness = data.project.brightness;
			editContrast = data.project.contrast;
			editSaturation = data.project.saturation;
			editVisibility = data.project.visibility_id || VISIBILITY_PUBLIC;
			editProjectType = data.project.project_type_id || PROJECT_TYPE_STANDARD;
			editBoothCountdown = data.project.booth_countdown || 0;
			slug = data.project.slug || '';
		} catch { data = null; }
	}

	onMount(load);

	function shareUrl(): string {
		if (slug) return `${window.location.origin}/p/${slug}`;
		return '';
	}

	let copied = $state(false);
	let saveVersion = $state(0); // bumped on save to bust QR cache

	function copyLink() {
		const url = shareUrl();
		if (!url) return;

		// navigator.clipboard requires secure context (HTTPS or localhost)
		// Fall back to execCommand for LAN IPs
		if (navigator.clipboard?.writeText) {
			navigator.clipboard.writeText(url).then(
				() => showCopied(),
				() => fallbackCopy(url)
			);
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
		showCopied();
	}

	function showCopied() {
		copied = true;
		setTimeout(() => copied = false, 2000);
	}

	async function handleSave() {
		saving = true;
		await updateProject(projectId, {
			name: editName,
			brightness: editBrightness,
			contrast: editContrast,
			saturation: editSaturation,
			visibility_id: editVisibility,
			project_type_id: editProjectType,
			booth_countdown: editBoothCountdown
		});
		saving = false;
		saved = true;
		await load();
		saveVersion++;
		setTimeout(() => saved = false, 3000);
	}

	async function handleDelete() {
		if (!confirm('Delete this project and all its photos?')) return;
		await deleteProject(projectId);
		goto('/admin/project');
	}
</script>

{#if !data}
	<p class="empty">Loading...</p>
{:else}
	<h2>Settings</h2>

	{#if saved}
		<div class="alert success">Settings saved</div>
	{/if}

	<label class="field">
		<span>Project Name</span>
		<input type="text" bind:value={editName} />
	</label>

	<label class="field">
		<span>Visibility</span>
		<select bind:value={editVisibility}>
			<option value={VISIBILITY_PUBLIC}>Public — shown on the guest project picker</option>
			<option value={VISIBILITY_HIDDEN}>Hidden — only accessible via direct link / QR code</option>
			<option value={VISIBILITY_PRIVATE}>Private — admin only, not accessible to guests</option>
		</select>
	</label>

	<label class="field">
		<span>Project Type</span>
		<select bind:value={editProjectType}>
			<option value={PROJECT_TYPE_STANDARD}>Standard — guests upload photos for admin review</option>
			<option value={PROJECT_TYPE_BOOTH}>Photo Booth — guests take photos with camera and print instantly</option>
		</select>
	</label>

	{#if editProjectType === PROJECT_TYPE_BOOTH}
		<label class="field">
			<span>Countdown Timer</span>
			<select bind:value={editBoothCountdown}>
				<option value={0}>None — take photo instantly</option>
				<option value={3}>3 seconds</option>
				<option value={5}>5 seconds</option>
			</select>
		</label>
	{/if}

	{#if editVisibility !== VISIBILITY_PRIVATE && slug}
		<div class="share-box card">
			<span class="share-label">Share Link</span>
			<div class="share-url">
				<a href={shareUrl()} target="_blank" class="share-link">{shareUrl()}</a>
				<button class="copy-btn ghost" onclick={copyLink}>
					{copied ? 'Copied!' : 'Copy'}
				</button>
			</div>
			<div class="qr-section">
				<img src="/api/qr/project/{projectId}?v={saveVersion}" alt="QR Code" class="qr-image" />
				<a href="/api/qr/project/{projectId}?v={saveVersion}" download="qr-{slug}.png" class="qr-download">Download QR Code</a>
			</div>
			<p class="share-hint">
				{#if editVisibility === VISIBILITY_HIDDEN}
					This project is only accessible via this link or QR code. It won't appear on the guest project picker.
				{:else}
					This project is public and also accessible via this link or QR code.
				{/if}
			</p>
		</div>
	{/if}

	<label class="slider-group">
		<span>Brightness: {editBrightness.toFixed(2)}</span>
		<input type="range" min="-1" max="1" step="0.05" bind:value={editBrightness} />
	</label>
	<label class="slider-group">
		<span>Contrast: {editContrast.toFixed(2)}</span>
		<input type="range" min="-1" max="1" step="0.05" bind:value={editContrast} />
	</label>
	<label class="slider-group">
		<span>Saturation: {editSaturation.toFixed(2)}</span>
		<input type="range" min="-1" max="1" step="0.05" bind:value={editSaturation} />
	</label>

	<button class="primary" style="width: 100%; margin-top: 12px;" onclick={handleSave} disabled={saving}>
		{saving ? 'Saving...' : 'Save Settings'}
	</button>

	<hr class="divider" />

	<h3>Danger Zone</h3>
	<button class="danger" style="width: 100%;" onclick={handleDelete}>
		Delete Project
	</button>
{/if}

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }
	h3 { font-size: 1rem; font-weight: 600; margin-bottom: 12px; color: var(--danger); }
	.empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
	.field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
	.field span { font-size: 0.8rem; color: var(--text-muted); font-weight: 500; }
	.slider-group { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
	.slider-group span { font-size: 0.8rem; color: var(--text-muted); }
	.slider-group input[type="range"] { width: 100%; min-height: auto; padding: 0; border: none; background: transparent; }
	.divider { border: none; border-top: 1px solid var(--border); margin: 32px 0; }

	.share-box {
		padding: 12px 16px;
		margin-bottom: 16px;
	}

	.share-label {
		display: block;
		font-size: 0.8rem;
		color: var(--text-muted);
		font-weight: 600;
		margin-bottom: 6px;
	}

	.share-url {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 6px;
	}

	.share-link {
		flex: 1;
		font-size: 0.8rem;
		color: var(--accent);
		word-break: break-all;
		font-family: monospace;
	}

	.copy-btn {
		padding: 4px 12px;
		font-size: 0.8rem;
		min-height: auto;
		white-space: nowrap;
	}

	.qr-section {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		margin: 12px 0;
		padding: 16px;
		background: white;
		border-radius: var(--radius-sm);
	}

	.qr-image {
		width: 200px;
		height: 200px;
		image-rendering: pixelated;
	}

	.qr-download {
		font-size: 0.8rem;
		color: var(--accent);
	}

	.share-hint {
		font-size: 0.75rem;
		color: var(--text-muted);
	}
</style>
