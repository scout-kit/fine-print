<script lang="ts">
	import { onMount } from 'svelte';
	import {
		getSettings, updateSettings, listPrinters,
		syncPrinters, updatePrinterEnabled, getPrinterSettings, updatePrinterMode,
		listFonts, uploadFont, deleteFont,
		type PrinterInfo, type PrinterAssignment, type Font
	} from '$lib/api';

	let settings: Record<string, string> = $state({});
	let printers: PrinterInfo[] = $state([]);
	let printerAssignments: PrinterAssignment[] = $state([]);
	let printerMode = $state('round_robin');
	let fonts: Font[] = $state([]);
	let saving = $state(false);
	let saved = $state(false);
	let error = $state('');

	// Editable fields
	let printerMedia = $state('4x6');
	let hotspotSSID = $state('');
	let hotspotPassword = $state('');
	let gatewayIP = $state('');

	async function load() {
		try {
			settings = await getSettings();
			printerMedia = settings['printer_media'] || '4x6';
			hotspotSSID = settings['hotspot_ssid'] || 'Fine Print';
			hotspotPassword = settings['hotspot_password'] || '';
			gatewayIP = settings['gateway_ip'] || '192.168.69.1';
		} catch { /* ignore */ }

		try { printers = await listPrinters(); } catch { /* CUPS may not be available */ }
		try {
			const ps = await getPrinterSettings();
			printerAssignments = ps.printers || [];
			printerMode = ps.mode || 'round_robin';
		} catch { /* ignore */ }
		try { fonts = await listFonts(); } catch { /* ignore */ }
	}

	onMount(load);

	async function handleSave() {
		saving = true;
		saved = false;
		error = '';
		try {
			await updateSettings({
				printer_media: printerMedia,
				hotspot_ssid: hotspotSSID,
				hotspot_password: hotspotPassword,
				gateway_ip: gatewayIP
			});
			saved = true;
			setTimeout(() => saved = false, 3000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save';
		}
		saving = false;
	}

	async function handleSyncPrinters() {
		printerAssignments = await syncPrinters();
	}

	async function handleTogglePrinter(name: string, enabled: boolean) {
		await updatePrinterEnabled(name, enabled);
		load();
	}

	async function handleModeChange() {
		await updatePrinterMode(printerMode);
	}

	async function handleFontUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.[0]) return;
		await uploadFont(input.files[0]);
		input.value = '';
		fonts = await listFonts();
	}

	async function handleFontDelete(id: number) {
		await deleteFont(id);
		fonts = await listFonts();
	}
</script>

<h2>Settings</h2>

{#if saved}
	<div class="alert success">Settings saved</div>
{/if}

{#if error}
	<div class="alert error">{error}</div>
{/if}

<section class="settings-section">
	<h3>Printers</h3>

	<div class="printer-controls">
		<button class="ghost" style="padding: 8px 16px; font-size: 0.875rem;" onclick={handleSyncPrinters}>
			Detect Printers
		</button>
		<label class="field" style="margin: 0; flex: 1;">
			<span>Assignment Mode</span>
			<select bind:value={printerMode} onchange={handleModeChange}>
				<option value="round_robin">Round Robin (auto)</option>
				<option value="manual">Manual (admin picks)</option>
			</select>
		</label>
	</div>

	{#if printerAssignments.length > 0}
		<div class="printer-list">
			{#each printerAssignments as pa}
				<div class="printer-row">
					<label class="printer-toggle">
						<input
							type="checkbox"
							checked={pa.enabled}
							onchange={() => handleTogglePrinter(pa.name, !pa.enabled)}
						/>
						<span>{pa.name}</span>
					</label>
					<span class="printer-status" class:enabled={pa.enabled}>
						{pa.enabled ? 'Enabled' : 'Disabled'}
					</span>
				</div>
			{/each}
		</div>
	{:else if printers.length > 0}
		<p class="hint">Found {printers.length} CUPS printer(s). Click "Detect Printers" to manage them.</p>
	{:else}
		<p class="hint">No printers detected. Connect a printer and click "Detect Printers".</p>
	{/if}

	<label class="field">
		<span>Media Size</span>
		<select bind:value={printerMedia}>
			<option value="4x6">4x6</option>
			<option value="Postcard">Postcard</option>
		</select>
	</label>
</section>

<section class="settings-section">
	<h3>Fonts</h3>
	<p class="hint">Upload TTF/OTF fonts for use in text overlays. System fonts are used by default.</p>

	{#if fonts.length > 0}
		<div class="font-list">
			{#each fonts as font}
				<div class="font-row">
					<span>{font.name}</span>
					<span class="font-file">{font.filename}</span>
					<button class="ghost" style="padding: 4px 10px; color: var(--danger); font-size: 0.8rem;" onclick={() => handleFontDelete(font.id)}>Remove</button>
				</div>
			{/each}
		</div>
	{/if}

	<label class="upload-font-btn ghost">
		Upload Font (TTF/OTF)
		<input type="file" accept=".ttf,.otf,.ttc" hidden onchange={handleFontUpload} />
	</label>
</section>

<section class="settings-section">
	<h3>Hotspot</h3>

	<label class="field">
		<span>WiFi SSID</span>
		<input type="text" bind:value={hotspotSSID} />
	</label>

	<label class="field">
		<span>WiFi Password (empty = open)</span>
		<input type="text" bind:value={hotspotPassword} placeholder="Leave empty for open network" />
	</label>

	<label class="field">
		<span>Gateway IP</span>
		<input type="text" bind:value={gatewayIP} />
	</label>
</section>

<button class="primary" style="width: 100%;" onclick={handleSave} disabled={saving}>
	{saving ? 'Saving...' : 'Save Settings'}
</button>

<style>
	h2 { font-size: 1.5rem; margin-bottom: 16px; }

	.settings-section {
		margin-bottom: 24px;
	}

	.settings-section h3 {
		font-size: 1rem;
		font-weight: 600;
		margin-bottom: 12px;
		padding-bottom: 8px;
		border-bottom: 1px solid var(--border);
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

	.hint {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin-bottom: 12px;
	}

	.printer-controls {
		display: flex;
		gap: 12px;
		align-items: flex-end;
		margin-bottom: 12px;
	}

	.printer-list, .font-list {
		margin-bottom: 12px;
	}

	.printer-row, .font-row {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--bg-surface);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		margin-bottom: 4px;
	}

	.printer-toggle {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
		cursor: pointer;
	}

	.printer-toggle input {
		width: auto;
		min-height: auto;
	}

	.printer-status {
		font-size: 0.75rem;
		color: var(--text-muted);
	}

	.printer-status.enabled {
		color: var(--success);
	}

	.font-row span:first-child {
		font-weight: 600;
		flex: 1;
	}

	.font-file {
		color: var(--text-muted);
		font-size: 0.8rem;
		flex: none !important;
		font-weight: normal !important;
	}

	.upload-font-btn {
		display: inline-block;
		padding: 8px 16px;
		font-size: 0.875rem;
		cursor: pointer;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		color: var(--text-muted);
	}
</style>
