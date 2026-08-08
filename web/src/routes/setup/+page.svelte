<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getSetupStatus, completeSetup, type PrinterInfo } from '$lib/api';

	let checking = $state(true);
	let loading = $state(false);
	let submitted = $state(false);
	let error = $state('');

	let adminPassword = $state('');
	let adminPasswordConfirm = $state('');
	let hotspotSSID = $state('Fine Print');
	let hotspotPassword = $state('');
	let printers: PrinterInfo[] = $state([]);
	let printerName = $state('');

	onMount(async () => {
		try {
			const s = await getSetupStatus();
			if (!s.needs_setup) {
				goto('/admin');
				return;
			}
			printers = s.printers ?? [];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load setup status';
		}
		checking = false;
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (adminPassword.length < 4) {
			error = 'Admin password must be at least 4 characters';
			return;
		}
		if (adminPassword !== adminPasswordConfirm) {
			error = 'Passwords do not match';
			return;
		}
		loading = true;
		try {
			await completeSetup({
				admin_password: adminPassword,
				hotspot_ssid: hotspotSSID,
				hotspot_password: hotspotPassword,
				printer_name: printerName
			});
			submitted = true;
			setTimeout(() => goto('/admin'), 1200);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Setup failed';
		}
		loading = false;
	}
</script>

<svelte:head>
	<title>Fine Print — First-run setup</title>
</svelte:head>

<div class="wizard">
	<h1>Welcome to Fine Print</h1>
	<p class="lede">A few quick settings and you'll be ready to print.</p>

	{#if checking}
		<p class="hint">Checking setup status…</p>
	{:else if submitted}
		<div class="alert success">Setup complete. Redirecting to admin…</div>
	{:else}
		{#if error}<div class="alert error">{error}</div>{/if}

		<form onsubmit={handleSubmit}>
			<section>
				<h2>Admin password <span class="req">required</span></h2>
				<p class="hint">Used to access the admin dashboard. Pick something you'll remember — there's no recovery flow.</p>

				<label class="field">
					<span>Password</span>
					<input type="password" bind:value={adminPassword} autocomplete="new-password" required />
				</label>
				<label class="field">
					<span>Confirm password</span>
					<input type="password" bind:value={adminPasswordConfirm} autocomplete="new-password" required />
				</label>
			</section>

			<section>
				<h2>WiFi hotspot <span class="opt">optional</span></h2>
				<p class="hint">Guests connect to this network to reach the kiosk. Leave the password blank for an open network.</p>

				<label class="field">
					<span>Network name (SSID)</span>
					<input type="text" bind:value={hotspotSSID} />
				</label>
				<label class="field">
					<span>Password</span>
					<input type="text" bind:value={hotspotPassword} placeholder="Leave empty for open network" />
				</label>
			</section>

			<section>
				<h2>Printer <span class="opt">optional</span></h2>
				<p class="hint">
					{#if printers.length === 0}
						No CUPS printers detected. You can configure one later in Settings.
					{:else}
						Select the printer to use for 4x6 prints. You can change this later.
					{/if}
				</p>

				{#if printers.length > 0}
					<label class="field">
						<span>Printer</span>
						<select bind:value={printerName}>
							<option value="">— skip for now —</option>
							{#each printers as p}
								<option value={p.name}>{p.name}{p.description ? ` — ${p.description}` : ''}</option>
							{/each}
						</select>
					</label>
				{/if}
			</section>

			<button class="primary" type="submit" disabled={loading}>
				{loading ? 'Saving…' : 'Finish setup'}
			</button>
		</form>
	{/if}
</div>

<style>
	.wizard {
		max-width: 520px;
		margin: 48px auto;
		padding: 24px;
	}

	h1 {
		font-size: 1.75rem;
		margin-bottom: 8px;
	}

	.lede {
		color: var(--text-muted);
		margin-bottom: 24px;
	}

	section {
		margin-bottom: 28px;
		padding-bottom: 20px;
		border-bottom: 1px solid var(--border);
	}

	section:last-of-type {
		border-bottom: none;
	}

	section h2 {
		font-size: 1rem;
		font-weight: 600;
		margin-bottom: 4px;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.req, .opt {
		font-size: 0.65rem;
		padding: 2px 6px;
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		font-weight: 500;
	}

	.req {
		background: var(--danger-bg, rgba(200, 60, 60, 0.15));
		color: var(--danger, #c83c3c);
	}

	.opt {
		background: var(--bg-surface, rgba(0, 0, 0, 0.05));
		color: var(--text-muted);
	}

	.hint {
		font-size: 0.85rem;
		color: var(--text-muted);
		margin-bottom: 12px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		margin-bottom: 12px;
	}

	.field span {
		font-size: 0.8rem;
		color: var(--text-muted);
		font-weight: 500;
	}

	button.primary {
		width: 100%;
	}
</style>
