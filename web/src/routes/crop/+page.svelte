<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import CropEditor from '$lib/CropEditor.svelte';
	import { saveTransform, previewUrl, photoStatus, type CropTransform } from '$lib/api';

	const photoId = $derived(Number(page.url.searchParams.get('id')));
	const returnUrl = $derived(page.url.searchParams.get('return') || '');
	let saving = $state(false);
	let error = $state('');
	let previewReady = $state(false);

	onMount(() => {
		if (!photoId) return;
		checkPreview();
	});

	async function checkPreview() {
		// Poll until preview endpoint returns 200
		for (let i = 0; i < 30; i++) {
			try {
				const res = await fetch(previewUrl(photoId), { method: 'HEAD' });
				if (res.ok) {
					previewReady = true;
					return;
				}
			} catch { /* ignore */ }
			await new Promise(r => setTimeout(r, 1000));
		}
		error = 'Preview is taking too long to generate. Try refreshing.';
	}

	async function handleSave(transform: CropTransform) {
		if (!photoId) return;
		saving = true;
		error = '';

		try {
			await saveTransform(photoId, transform);
			if (returnUrl) {
				goto(returnUrl);
			} else {
				goto(`/status?id=${photoId}`);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save';
			saving = false;
		}
	}
</script>

<div class="container">
	<header>
		<a href={returnUrl || '/'} class="back">&larr; Back</a>
		<h2>Crop Photo</h2>
	</header>

	{#if !photoId}
		<div class="card" style="text-align: center;">
			<p>No photo selected</p>
			<a href="/">Upload a photo</a>
		</div>
	{:else if error}
		<div class="alert error">{error}</div>
	{:else if !previewReady}
		<div style="text-align: center; padding: 48px 0;">
			<div class="spinner"></div>
			<p style="margin-top: 12px; color: var(--text-muted);">Preparing your photo...</p>
		</div>
	{:else if saving}
		<div style="text-align: center; padding: 48px 0;">
			<div class="spinner"></div>
			<p style="margin-top: 12px;">Submitting...</p>
		</div>
	{:else}
		<CropEditor
			imageUrl={previewUrl(photoId)}
			onSave={handleSave}
		/>
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
