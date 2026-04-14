const BASE = '/api';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const opts: RequestInit = {
		method,
		headers: {}
	};

	if (body instanceof FormData) {
		opts.body = body;
	} else if (body !== undefined) {
		(opts.headers as Record<string, string>)['Content-Type'] = 'application/json';
		opts.body = JSON.stringify(body);
	}

	const res = await fetch(`${BASE}${path}`, opts);

	if (!res.ok) {
		const err = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(err.error || `Request failed: ${res.status}`);
	}

	if (res.status === 204) return undefined as T;
	return res.json();
}

// Guest API
export function uploadPhoto(file: File, projectId: number): Promise<{ id: number; status: string }> {
	const form = new FormData();
	form.append('photo', file);
	form.append('project_id', String(projectId));
	return request('POST', '/photos', form);
}

export function photoStatus(id: number): Promise<{ id: number; status: string }> {
	return request('GET', `/photos/${id}/status`);
}

export function previewUrl(id: number): string {
	return `${BASE}/photos/${id}/preview`;
}

export function saveTransform(id: number, transform: CropTransform): Promise<{ status: string }> {
	return request('POST', `/photos/${id}/transform`, transform);
}

export function getEdits(id: number): Promise<EditsResponse> {
	return request('GET', `/photos/${id}/edits`);
}

export function saveEdits(id: number, edits: EditState): Promise<{ status: string }> {
	return request('POST', `/photos/${id}/edits`, edits);
}

export function reprintPhoto(id: number, clearCache: boolean = false): Promise<{ status: string }> {
	return request('POST', `/admin/photos/${id}/reprint`, { clear_cache: clearCache });
}

export function listProjectsPublic(): Promise<Project[]> {
	return request('GET', '/projects');
}

export function getProjectPublic(id: number): Promise<ProjectResponse> {
	return request('GET', `/projects/${id}`);
}

// Admin API
export function adminLogin(password: string): Promise<{ status: string }> {
	return request('POST', '/admin/login', { password });
}

export function adminSession(): Promise<{ status: string }> {
	return request('GET', '/admin/session');
}

export function adminLogout(): Promise<{ status: string }> {
	return request('POST', '/admin/logout');
}

export function listPhotos(status?: string, projectId?: number): Promise<Photo[]> {
	const params = new URLSearchParams();
	if (status) params.set('status', status);
	if (projectId) params.set('project_id', String(projectId));
	const query = params.toString();
	return request('GET', `/admin/photos${query ? '?' + query : ''}`);
}

export function approvePhoto(id: number): Promise<{ status: string }> {
	return request('POST', `/admin/photos/${id}/approve`);
}

export function rejectPhoto(id: number): Promise<{ status: string }> {
	return request('POST', `/admin/photos/${id}/reject`);
}

export function unapprovePhoto(id: number): Promise<{ status: string }> {
	return request('POST', `/admin/photos/${id}/unapprove`);
}

export function deletePhoto(id: number): Promise<{ status: string }> {
	return request('DELETE', `/admin/photos/${id}`);
}

export function listQueue(): Promise<QueueResponse> {
	return request('GET', '/admin/queue');
}

export function pauseQueue(): Promise<{ status: string }> {
	return request('POST', '/admin/queue/pause');
}

export function resumeQueue(): Promise<{ status: string }> {
	return request('POST', '/admin/queue/resume');
}

export function retryJob(id: number): Promise<{ status: string }> {
	return request('POST', `/admin/queue/${id}/retry`);
}

export function cancelJob(id: number): Promise<{ status: string }> {
	return request('POST', `/admin/queue/${id}/cancel`);
}

export function listProjects(): Promise<Project[]> {
	return request('GET', '/admin/projects');
}

export function createProject(name: string, visibilityId: number = VISIBILITY_PUBLIC): Promise<Project> {
	return request('POST', '/admin/projects', { name, visibility_id: visibilityId });
}

export function getProjectBySlug(slug: string): Promise<ProjectResponse> {
	return request('GET', `/projects/s/${slug}`);
}

export function updateProject(id: number, data: Partial<Project>): Promise<Project> {
	return request('PUT', `/admin/projects/${id}`, data);
}

export function deleteProject(id: number): Promise<{ status: string }> {
	return request('DELETE', `/admin/projects/${id}`);
}

export function copyProject(id: number, name: string, visibilityId: number): Promise<Project> {
	return request('POST', `/admin/projects/${id}/copy`, { name, visibility_id: visibilityId });
}

export function copyTemplateOrientation(projectId: number, from: number, to: number): Promise<{ status: string }> {
	return request('POST', `/admin/projects/${projectId}/copy-template`, { from, to });
}

export function uploadOverlay(projectId: number, file: File, orientationId: number = 1): Promise<Overlay> {
	const form = new FormData();
	form.append('overlay', file);
	form.append('orientation_id', String(orientationId));
	return request('POST', `/admin/projects/${projectId}/overlay`, form);
}

export function updateOverlayPosition(id: number, data: { x: number; y: number; width: number; height: number; opacity: number }): Promise<{ status: string }> {
	return request('PUT', `/admin/overlays/${id}`, data);
}

export function deleteOverlay(id: number): Promise<{ status: string }> {
	return request('DELETE', `/admin/overlays/${id}`);
}

export function createTextOverlay(projectId: number, data: { text: string; font_family?: string; font_size?: number; color?: string; x?: number; y?: number; opacity?: number; orientation_id?: number }): Promise<TextOverlay> {
	return request('POST', `/admin/projects/${projectId}/text-overlay`, data);
}

export function updateTextOverlay(id: number, data: Partial<TextOverlay>): Promise<{ status: string }> {
	return request('PUT', `/admin/text-overlays/${id}`, data);
}

export function deleteTextOverlay(id: number): Promise<{ status: string }> {
	return request('DELETE', `/admin/text-overlays/${id}`);
}

export function getProject(id: number): Promise<ProjectResponse> {
	return request('GET', `/admin/projects/${id}`);
}

export function listPrinters(): Promise<PrinterInfo[]> {
	return request('GET', '/admin/printers');
}

export function syncPrinters(): Promise<PrinterAssignment[]> {
	return request('POST', '/admin/printers/sync');
}

export function updatePrinterEnabled(name: string, enabled: boolean): Promise<{ status: string }> {
	return request('PUT', '/admin/printers/enabled', { name, enabled });
}

export function getPrinterSettings(): Promise<{ mode: string; printers: PrinterAssignment[] }> {
	return request('GET', '/admin/printers/settings');
}

export function updatePrinterMode(mode: string): Promise<{ status: string }> {
	return request('PUT', '/admin/printers/mode', { mode });
}

// Fonts
export function listFonts(): Promise<Font[]> {
	return request('GET', '/fonts');
}

export interface SystemFont {
	name: string;
	path: string;
	css_name: string;
}

export function listAvailableFonts(): Promise<SystemFont[]> {
	return request('GET', '/fonts/available');
}

export function uploadFont(file: File): Promise<Font> {
	const form = new FormData();
	form.append('font', file);
	return request('POST', '/admin/fonts', form);
}

export function deleteFont(id: number): Promise<{ status: string }> {
	return request('DELETE', `/admin/fonts/${id}`);
}

// Gallery
export function getGallery(): Promise<GalleryPhoto[]> {
	return request('GET', '/gallery');
}

export function deleteOwnPhoto(id: number): Promise<{ status: string }> {
	return request('DELETE', `/photos/${id}`);
}

// Download URLs
export function downloadOriginalUrl(id: number): string {
	return `${BASE}/photos/${id}/download/original`;
}

export function downloadRenderedUrl(id: number): string {
	return `${BASE}/photos/${id}/download/rendered`;
}

export function renderPreviewUrl(id: number): string {
	return `${BASE}/photos/${id}/render`;
}

export function exportProjectUrl(projectId: number): string {
	return `${BASE}/admin/export/${projectId}`;
}

export async function exportPhotos(photoIds: number[]): Promise<void> {
	const res = await fetch(`${BASE}/admin/export/photos`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ photo_ids: photoIds })
	});
	if (!res.ok) throw new Error('Export failed');
	const blob = await res.blob();
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = `fine-print-selection.zip`;
	a.click();
	URL.revokeObjectURL(url);
}

export function getSettings(): Promise<Record<string, string>> {
	return request('GET', '/admin/settings');
}

export function updateSettings(settings: Record<string, string>): Promise<{ status: string }> {
	return request('PUT', '/admin/settings', settings);
}

// Types
export interface CropTransform {
	crop_x: number;
	crop_y: number;
	crop_width: number;
	crop_height: number;
	rotation: number;
}

export interface EditState {
	crop_x: number;
	crop_y: number;
	crop_width: number;
	crop_height: number;
	rotation: number;
	brightness: number | null;
	contrast: number | null;
	saturation: number | null;
	overlay_overrides: unknown[];
	text_overrides: unknown[];
	copies: number;
}

export interface EditsResponse {
	transform?: CropTransform;
	overrides?: {
		brightness: number | null;
		contrast: number | null;
		saturation: number | null;
		overlay_overrides: unknown[] | null;
		text_overrides: unknown[] | null;
	};
	project?: {
		brightness: number;
		contrast: number;
		saturation: number;
	};
	copies: number;
}

export interface Photo {
	id: number;
	project_id: number;
	session_id: string;
	status_id: number;
	preview_key: string | null;
	copies: number;
	created_at: string;
}

export interface Project {
	id: number;
	name: string;
	brightness: number;
	contrast: number;
	saturation: number;
	visibility_id: number;
	slug: string | null;
	created_at: string;
}

// Visibility constants
export const VISIBILITY_PUBLIC = 1;
export const VISIBILITY_HIDDEN = 2;
export const VISIBILITY_PRIVATE = 3;

export const VISIBILITY_LABELS: Record<number, string> = {
	1: 'Public',
	2: 'Hidden (link only)',
	3: 'Private (admin only)'
};

export const ORIENTATION_LANDSCAPE = 1;
export const ORIENTATION_PORTRAIT = 2;

export const PROJECT_TYPE_STANDARD = 1;
export const PROJECT_TYPE_BOOTH = 2;

export const PROJECT_TYPE_LABELS: Record<number, string> = {
	1: 'Standard',
	2: 'Photo Booth'
};

export function boothPrint(id: number, edits: EditState): Promise<{ status: string }> {
	return request('POST', `/photos/${id}/booth-print`, edits);
}

export interface Overlay {
	id: number;
	project_id: number;
	filename: string;
	x: number;
	y: number;
	width: number;
	height: number;
	opacity: number;
	orientation_id: number;
}

export interface TextOverlay {
	id: number;
	project_id: number;
	text: string;
	font_family: string;
	font_size: number;
	color: string;
	x: number;
	y: number;
	opacity: number;
	orientation_id: number;
}

export interface ProjectResponse {
	project: Project;
	overlays: Overlay[];
	text_overlays: TextOverlay[];
}

export interface PrintJob {
	id: number;
	photo_id: number;
	status_id: number;
	position: number;
	printer_name: string | null;
	error_msg: string | null;
	attempts: number;
}

export interface QueueResponse {
	jobs: PrintJob[];
	paused: boolean;
}

export interface PrinterInfo {
	name: string;
	device: string;
	accept_jobs: boolean;
}

export interface PrinterAssignment {
	id: number;
	name: string;
	enabled: boolean;
}

export interface Font {
	id: number;
	name: string;
	filename: string;
	storage_key: string;
}

export interface GalleryPhoto {
	id: number;
	status_id: number;
	status: string;
	has_preview: boolean;
	has_render: boolean;
	session_id: string;
	created_at: string;
}

// Status helpers
const PHOTO_STATUS: Record<number, string> = {
	1: 'uploaded', 2: 'approved', 3: 'queued',
	4: 'printing', 5: 'printed', 6: 'failed', 7: 'rejected'
};

const JOB_STATUS: Record<number, string> = {
	1: 'queued', 2: 'printing', 3: 'printed', 4: 'failed', 5: 'canceled'
};

export function photoStatusName(id: number): string {
	return PHOTO_STATUS[id] || 'unknown';
}

export function jobStatusName(id: number): string {
	return JOB_STATUS[id] || 'unknown';
}
