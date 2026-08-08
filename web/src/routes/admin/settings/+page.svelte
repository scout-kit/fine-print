<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		getSettings, updateSettings, changeAdminPassword, restartService,
		listPrinters, syncPrinters, updatePrinterEnabled, getPrinterSettings, updatePrinterMode,
		listFonts, uploadFont, deleteFont,
		getStorage, backupDownloadURL, restoreBackup,
		type SettingField, type PrinterInfo, type PrinterAssignment, type Font,
		type DiskUsage
	} from '$lib/api';
	import { createSSE, type SSEConnection } from '$lib/sse';

	// Settings field keys — mirror internal/settings/settings.go.
	const K = {
		HotspotEnabled:     'hotspot_enabled',
		HotspotSSID:        'hotspot_ssid',
		HotspotPassword:    'hotspot_password',
		HotspotInterface:   'hotspot_interface',
		HotspotSubnet:      'hotspot_subnet',
		HotspotGateway:     'gateway_ip',
		DNSEnabled:         'dns_enabled',
		DNSPort:            'dns_port',
		PrinterName:        'printer_name',
		PrinterMedia:       'printer_media',
		PrinterAutoQueue:   'printer_auto_queue',
		ImagingMaxUpload:   'imaging_max_upload_pixels',
		ImagingPreviewW:    'imaging_preview_max_width',
		ImagingPrintWidth:  'imaging_print_width',
		ImagingPrintHeight: 'imaging_print_height',
		ImagingJPEGQuality: 'imaging_jpeg_quality',
		DiskGuardMinFreeBytes:      'diskguard_min_free_bytes',
		PrinterMonitorIntervalSecs: 'printer_monitor_interval_seconds'
	} as const;

	let fields: Record<string, SettingField> = $state({});
	let pending: Record<string, string> = $state({});

	let printers: PrinterInfo[] = $state([]);
	let printerAssignments: PrinterAssignment[] = $state([]);
	let printerMode = $state('round_robin');
	let fonts: Font[] = $state([]);

	let saving = $state(false);
	let saved = $state(false);
	let error = $state('');
	let restartPending = $state(false);
	let restarting = $state(false);

	// Password change
	let pwCurrent = $state('');
	let pwNew = $state('');
	let pwConfirm = $state('');
	let pwSaving = $state(false);
	let pwSaved = $state(false);
	let pwError = $state('');

	let sse: SSEConnection | null = null;

	// Storage + backup
	let storage: DiskUsage | null = $state(null);
	let restoreFile: File | null = $state(null);
	let restoring = $state(false);
	let restoreMsg = $state('');

	async function loadStorage() {
		try {
			const s = await getStorage();
			storage = s.enabled ? (s.usage ?? null) : null;
		} catch { /* ignore */ }
	}

	function bytesToGB(n: number): string {
		return (n / 1024 / 1024 / 1024).toFixed(2);
	}

	async function handleRestore() {
		if (!restoreFile) return;
		if (!confirm('Restore will replace the current database and photos. Existing data is moved aside as .bak files. Continue?')) return;
		restoring = true;
		restoreMsg = '';
		try {
			const r = await restoreBackup(restoreFile);
			restoreMsg = r.message || 'Restore complete.';
			if (r.requires_restart) restartPending = true;
		} catch (e) {
			restoreMsg = e instanceof Error ? e.message : 'Restore failed';
		}
		restoring = false;
	}

	async function loadSettings() {
		try {
			const res = await getSettings();
			const next: Record<string, SettingField> = {};
			for (const f of res.fields) next[f.key] = f;
			fields = next;
			pending = {};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load settings';
		}
	}

	async function load() {
		await loadSettings();
		await loadStorage();
		try { printers = await listPrinters(); } catch { /* CUPS may not be available */ }
		try {
			const ps = await getPrinterSettings();
			printerAssignments = ps.printers || [];
			printerMode = ps.mode || 'round_robin';
		} catch { /* ignore */ }
		try { fonts = await listFonts(); } catch { /* ignore */ }
	}

	onMount(() => {
		load();
		sse = createSSE('/api/admin/events');
		sse.state.subscribe((s) => {
			if (s.lastEvent?.type === 'settings_changed') {
				loadSettings();
			}
		});
	});

	onDestroy(() => sse?.close());

	function fieldValue(key: string): string {
		if (key in pending) return pending[key];
		return fields[key]?.value ?? '';
	}

	function setField(key: string, value: string) {
		pending[key] = value;
	}

	function fieldBool(key: string): boolean {
		return fieldValue(key) === 'true';
	}

	function setBool(key: string, on: boolean) {
		setField(key, on ? 'true' : 'false');
	}

	function dirty(): boolean {
		return Object.keys(pending).length > 0;
	}

	async function handleSave() {
		if (!dirty()) return;
		saving = true;
		saved = false;
		error = '';
		try {
			const res = await updateSettings({ ...pending });
			if (res.requires_restart) restartPending = true;
			saved = true;
			pending = {};
			await loadSettings();
			setTimeout(() => saved = false, 3000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save';
		}
		saving = false;
	}

	async function handleRestart() {
		if (!confirm('Restart the Fine Print service now? Any in-flight prints will pause briefly.')) return;
		restarting = true;
		try {
			await restartService();
			// Server will go away; poll for it to come back.
			setTimeout(() => window.location.reload(), 3000);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to restart';
			restarting = false;
		}
	}

	async function handlePasswordSave() {
		pwError = '';
		pwSaved = false;
		if (pwNew !== pwConfirm) {
			pwError = 'New passwords do not match';
			return;
		}
		if (pwNew.length < 4) {
			pwError = 'New password must be at least 4 characters';
			return;
		}
		pwSaving = true;
		try {
			await changeAdminPassword(pwCurrent, pwNew);
			pwSaved = true;
			pwCurrent = '';
			pwNew = '';
			pwConfirm = '';
			setTimeout(() => pwSaved = false, 3000);
		} catch (e) {
			pwError = e instanceof Error ? e.message : 'Failed to change password';
		}
		pwSaving = false;
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

{#if saved}<div class="alert success">Settings saved</div>{/if}
{#if error}<div class="alert error">{error}</div>{/if}

{#if restartPending}
	<div class="alert warning restart-banner">
		<span>Some changes require a service restart to take effect.</span>
		<button class="primary" onclick={handleRestart} disabled={restarting}>
			{restarting ? 'Restarting…' : 'Restart Now'}
		</button>
	</div>
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
		<select value={fieldValue(K.PrinterMedia)} onchange={(e) => setField(K.PrinterMedia, (e.currentTarget as HTMLSelectElement).value)}>
			<option value="4x6">4x6</option>
			<option value="Postcard">Postcard</option>
		</select>
	</label>

	<label class="field">
		<span>Default Printer (CUPS name)</span>
		<input type="text" value={fieldValue(K.PrinterName)} oninput={(e) => setField(K.PrinterName, (e.currentTarget as HTMLInputElement).value)} placeholder="empty = auto-select" />
	</label>

	<label class="toggle-row">
		<input type="checkbox" checked={fieldBool(K.PrinterAutoQueue)} onchange={(e) => setBool(K.PrinterAutoQueue, (e.currentTarget as HTMLInputElement).checked)} />
		<span>Auto-queue approved photos</span>
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
	<h3>Hotspot <span class="restart-tag">restart required</span></h3>

	<label class="toggle-row">
		<input type="checkbox" checked={fieldBool(K.HotspotEnabled)} onchange={(e) => setBool(K.HotspotEnabled, (e.currentTarget as HTMLInputElement).checked)} />
		<span>Enable WiFi hotspot on startup</span>
	</label>

	<label class="field">
		<span>WiFi SSID</span>
		<input type="text" value={fieldValue(K.HotspotSSID)} oninput={(e) => setField(K.HotspotSSID, (e.currentTarget as HTMLInputElement).value)} />
	</label>

	<label class="field">
		<span>WiFi Password (empty = open)</span>
		<input type="text" value={fieldValue(K.HotspotPassword)} oninput={(e) => setField(K.HotspotPassword, (e.currentTarget as HTMLInputElement).value)} placeholder="Leave empty for open network" />
	</label>

	<label class="field">
		<span>Network Interface</span>
		<input type="text" value={fieldValue(K.HotspotInterface)} oninput={(e) => setField(K.HotspotInterface, (e.currentTarget as HTMLInputElement).value)} placeholder="e.g. wlan0, en0" />
	</label>

	<label class="field">
		<span>Subnet</span>
		<input type="text" value={fieldValue(K.HotspotSubnet)} oninput={(e) => setField(K.HotspotSubnet, (e.currentTarget as HTMLInputElement).value)} placeholder="192.168.69.0/24" />
	</label>

	<label class="field">
		<span>Gateway IP</span>
		<input type="text" value={fieldValue(K.HotspotGateway)} oninput={(e) => setField(K.HotspotGateway, (e.currentTarget as HTMLInputElement).value)} />
	</label>
</section>

<section class="settings-section">
	<h3>DNS <span class="restart-tag">restart required</span></h3>

	<label class="toggle-row">
		<input type="checkbox" checked={fieldBool(K.DNSEnabled)} onchange={(e) => setBool(K.DNSEnabled, (e.currentTarget as HTMLInputElement).checked)} />
		<span>Enable captive-portal DNS hijack</span>
	</label>

	<label class="field">
		<span>DNS Port</span>
		<input type="number" value={fieldValue(K.DNSPort)} oninput={(e) => setField(K.DNSPort, (e.currentTarget as HTMLInputElement).value)} />
	</label>
</section>

<section class="settings-section">
	<h3>Imaging <span class="restart-tag">restart required</span></h3>
	<p class="hint">Values control the print render pipeline. 1800x1200 is 300dpi for 4x6.</p>

	<label class="field">
		<span>Print Width (pixels)</span>
		<input type="number" value={fieldValue(K.ImagingPrintWidth)} oninput={(e) => setField(K.ImagingPrintWidth, (e.currentTarget as HTMLInputElement).value)} />
	</label>

	<label class="field">
		<span>Print Height (pixels)</span>
		<input type="number" value={fieldValue(K.ImagingPrintHeight)} oninput={(e) => setField(K.ImagingPrintHeight, (e.currentTarget as HTMLInputElement).value)} />
	</label>

	<label class="field">
		<span>Preview Max Width (pixels)</span>
		<input type="number" value={fieldValue(K.ImagingPreviewW)} oninput={(e) => setField(K.ImagingPreviewW, (e.currentTarget as HTMLInputElement).value)} />
	</label>

	<label class="field">
		<span>JPEG Quality (1–100)</span>
		<input type="number" min="1" max="100" value={fieldValue(K.ImagingJPEGQuality)} oninput={(e) => setField(K.ImagingJPEGQuality, (e.currentTarget as HTMLInputElement).value)} />
	</label>

	<label class="field">
		<span>Max Upload Dimension (pixels)</span>
		<input type="number" value={fieldValue(K.ImagingMaxUpload)} oninput={(e) => setField(K.ImagingMaxUpload, (e.currentTarget as HTMLInputElement).value)} />
	</label>
</section>

<section class="settings-section">
	<h3>Reliability</h3>

	<label class="field">
		<span>Disk-space floor (bytes). Uploads are refused below this. Default 2 GB.</span>
		<input type="number" min="0" value={fieldValue(K.DiskGuardMinFreeBytes)} oninput={(e) => setField(K.DiskGuardMinFreeBytes, (e.currentTarget as HTMLInputElement).value)} placeholder="2147483648" />
	</label>

	<label class="field">
		<span>Printer check interval (seconds) <span class="restart-tag">restart required</span></span>
		<input type="number" min="5" max="3600" value={fieldValue(K.PrinterMonitorIntervalSecs)} oninput={(e) => setField(K.PrinterMonitorIntervalSecs, (e.currentTarget as HTMLInputElement).value)} placeholder="30" />
	</label>
</section>

<button class="primary save-btn" onclick={handleSave} disabled={saving || !dirty()}>
	{saving ? 'Saving…' : dirty() ? 'Save Settings' : 'No unsaved changes'}
</button>

<section class="settings-section">
	<h3>Admin Password</h3>

	{#if pwSaved}<div class="alert success">Password updated</div>{/if}
	{#if pwError}<div class="alert error">{pwError}</div>{/if}

	<label class="field">
		<span>Current Password</span>
		<input type="password" bind:value={pwCurrent} autocomplete="current-password" />
	</label>

	<label class="field">
		<span>New Password</span>
		<input type="password" bind:value={pwNew} autocomplete="new-password" />
	</label>

	<label class="field">
		<span>Confirm New Password</span>
		<input type="password" bind:value={pwConfirm} autocomplete="new-password" />
	</label>

	<button class="primary" onclick={handlePasswordSave} disabled={pwSaving || !pwCurrent || !pwNew}>
		{pwSaving ? 'Updating…' : 'Change Password'}
	</button>
</section>

<section class="settings-section">
	<h3>Storage</h3>

	{#if storage}
		<div class="storage-summary" class:warn={storage.warn_active} class:critical={!storage.above_min_free}>
			<div class="usage-bar" style="--frac: {storage.used_fraction * 100}%"></div>
			<div class="usage-text">
				<strong>{(storage.used_fraction * 100).toFixed(0)}% used</strong>
				— {bytesToGB(storage.used_bytes)} GB of {bytesToGB(storage.total_bytes)} GB,
				{bytesToGB(storage.free_bytes)} GB free
			</div>
			{#if storage.message}
				<div class="usage-message">{storage.message}</div>
			{/if}
		</div>
	{:else}
		<p class="hint">Storage stats unavailable.</p>
	{/if}
</section>

<section class="settings-section">
	<h3>Backup &amp; Restore</h3>
	<p class="hint">
		Backup contains the database plus original uploads, overlays, and fonts.
		Rendered/preview files are excluded — they regenerate automatically.
	</p>

	<a class="primary download-link" href={backupDownloadURL()} download>
		Download backup (.tar.gz)
	</a>

	<label class="field restore-field">
		<span>Restore from backup file</span>
		<input type="file" accept=".tar.gz,.tgz,application/gzip" onchange={(e) => { restoreFile = (e.currentTarget as HTMLInputElement).files?.[0] ?? null; }} />
	</label>
	<button class="primary" onclick={handleRestore} disabled={!restoreFile || restoring}>
		{restoring ? 'Restoring…' : 'Restore'}
	</button>
	{#if restoreMsg}
		<div class="alert {restoreMsg.toLowerCase().includes('fail') ? 'error' : 'success'}">{restoreMsg}</div>
	{/if}
</section>

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
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.restart-tag {
		font-size: 0.65rem;
		font-weight: 500;
		padding: 2px 6px;
		border-radius: 4px;
		background: var(--warning-bg, rgba(200, 140, 0, 0.15));
		color: var(--warning, #c88c00);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.restart-banner {
		display: flex;
		align-items: center;
		gap: 12px;
		justify-content: space-between;
	}

	.restart-banner button {
		flex: none;
		padding: 8px 16px;
	}

	.save-btn {
		width: 100%;
		margin-bottom: 24px;
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

	.toggle-row {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 12px;
		cursor: pointer;
	}

	.toggle-row input {
		width: auto;
		min-height: auto;
	}

	.toggle-row span {
		font-size: 0.9rem;
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

	.storage-summary {
		padding: 12px;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--bg-surface);
	}

	.storage-summary.warn {
		border-color: var(--warning, #c88c00);
		background: var(--warning-bg, rgba(200, 140, 0, 0.08));
	}

	.storage-summary.critical {
		border-color: var(--danger, #c83c3c);
		background: var(--danger-bg, rgba(200, 60, 60, 0.08));
	}

	.usage-bar {
		height: 8px;
		background: var(--border);
		border-radius: 4px;
		position: relative;
		overflow: hidden;
		margin-bottom: 8px;
	}

	.usage-bar::before {
		content: "";
		position: absolute;
		inset: 0;
		width: var(--frac);
		background: var(--accent);
	}

	.storage-summary.warn .usage-bar::before {
		background: var(--warning, #c88c00);
	}

	.storage-summary.critical .usage-bar::before {
		background: var(--danger, #c83c3c);
	}

	.usage-text {
		font-size: 0.85rem;
	}

	.usage-message {
		margin-top: 8px;
		font-size: 0.8rem;
		color: var(--text-muted);
	}

	.download-link {
		display: inline-block;
		text-decoration: none;
		text-align: center;
		margin-bottom: 12px;
	}

	.restore-field input[type="file"] {
		padding: 6px 0;
	}
</style>
