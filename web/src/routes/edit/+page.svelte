<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ImageEditor from '$lib/ImageEditor.svelte';
	import { previewUrl } from '$lib/api';

	const photoId = $derived(Number(page.url.searchParams.get('id')));
	const returnUrl = $derived(page.url.searchParams.get('return') || '');
	let previewReady = $state(false);
	let error = $state('');

	onMount(() => {
		if (!photoId) return;
		checkPreview();
	});

	async function checkPreview() {
		for (let i = 0; i < 30; i++) {
			try {
				const res = await fetch(previewUrl(photoId), { method: 'HEAD' });
				if (res.ok) { previewReady = true; return; }
			} catch { /* ignore */ }
			await new Promise(r => setTimeout(r, 1000));
		}
		error = 'Preview is taking too long. Try refreshing.';
	}

	function handleSave() {
		if (returnUrl) {
			goto(returnUrl);
		} else {
			// Go back to where the user came from
			history.back();
		}
	}
</script>

<div class="container">
	<header>
		{#if returnUrl}
			<a href={returnUrl} class="back">&larr; Back</a>
		{:else}
			<button class="back" onclick={() => history.back()}>&larr; Back</button>
		{/if}
		<h2>Edit Photo</h2>
	</header>

	{#if !photoId}
		<div class="card" style="text-align: center;">
			<p>No photo selected</p>
			<a href="/">Upload a photo</a>
		</div>
	{:else if error}
		<div class="alert error">{error}</div>
	{:else if !previewReady}
		<div class="loading">
			<div class="spinner"></div>
			<p>Preparing your photo...</p>
		</div>
	{:else}
		<ImageEditor {photoId} onSave={handleSave} />
	{/if}
</div>

<style>
	header { display: flex; align-items: center; gap: 16px; padding: 16px 0; }
	.back { color: var(--text-muted); font-size: 0.875rem; }
	h2 { font-size: 1.25rem; font-weight: 600; }
	.loading { text-align: center; padding: 48px 0; }
	.loading p { margin-top: 12px; color: var(--text-muted); }
	.spinner { width: 40px; height: 40px; border: 3px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: spin 0.8s linear infinite; margin: 0 auto; }
	@keyframes spin { to { transform: rotate(360deg); } }
</style>
