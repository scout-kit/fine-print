/**
 * Display helpers for photo capture metadata.
 *
 * `taken_at` is always populated by the backend: it's the EXIF capture time
 * when the file carried one, and the upload time otherwise. `taken_at_source`
 * says which, so the UI can label a fallback instead of implying the camera
 * recorded it.
 */

import type { TakenAtSource } from '$lib/api';

/** The capture-metadata fields any photo-shaped object may carry. */
export interface PhotoMetaFields {
	created_at: string;
	taken_at?: string;
	taken_at_source?: TakenAtSource;
	taken_at_exif?: string | null;
	camera_label?: string;
	original_width?: number | null;
	original_height?: number | null;
	file_size?: number | null;
	mime_type?: string | null;
}

/** True when the timestamp came from the file's own EXIF data. */
export function isExifDate(photo: PhotoMetaFields): boolean {
	return photo.taken_at_source === 'exif';
}

/**
 * The timestamp to show. Falls back to created_at for callers holding an
 * older/partial photo object that predates these fields.
 */
export function takenAt(photo: PhotoMetaFields): Date {
	return new Date(photo.taken_at || photo.created_at);
}

/** "March 14, 2026 at 2:31 PM" — the full capture moment. */
export function formatTakenAt(photo: PhotoMetaFields): string {
	return takenAt(photo).toLocaleString(undefined, {
		year: 'numeric',
		month: 'long',
		day: 'numeric',
		hour: 'numeric',
		minute: '2-digit'
	});
}

/** "Mar 14, 2:31 PM" — compact form for list rows. */
export function formatTakenAtShort(photo: PhotoMetaFields): string {
	return takenAt(photo).toLocaleString(undefined, {
		month: 'short',
		day: 'numeric',
		hour: 'numeric',
		minute: '2-digit'
	});
}

/**
 * How the timestamp was obtained, phrased for display. Returns an empty
 * string when it's a genuine EXIF capture time and needs no qualifier.
 */
export function takenAtSourceLabel(photo: PhotoMetaFields): string {
	return isExifDate(photo) ? '' : 'upload time — no capture date in file';
}

/** "4032 × 3024" or empty when dimensions are unknown. */
export function formatDimensions(photo: PhotoMetaFields): string {
	const { original_width: w, original_height: h } = photo;
	if (!w || !h) return '';
	return `${w} × ${h}`;
}

/** Human-readable byte size, or empty when unknown. */
export function formatFileSize(photo: PhotoMetaFields): string {
	const bytes = photo.file_size;
	if (!bytes || bytes <= 0) return '';
	const units = ['B', 'KB', 'MB', 'GB'];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit++;
	}
	// Whole numbers for bytes/KB, one decimal above that.
	return `${unit >= 2 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}

/** One label/value pair for the metadata panel. */
export interface MetaRow {
	label: string;
	value: string;
	/** Set on rows that need a caveat, e.g. a fallback timestamp. */
	note?: string;
}

/**
 * The metadata rows worth showing for a photo, omitting anything unknown.
 * Capture date comes first — it's the field that drives date overlays.
 */
export function metaRows(photo: PhotoMetaFields): MetaRow[] {
	const rows: MetaRow[] = [
		{ label: 'Taken', value: formatTakenAt(photo), note: takenAtSourceLabel(photo) || undefined }
	];

	// Show the upload time separately only when it isn't already the "Taken"
	// value, so a fallback photo doesn't display the same timestamp twice.
	if (isExifDate(photo)) {
		rows.push({
			label: 'Uploaded',
			value: new Date(photo.created_at).toLocaleString(undefined, {
				month: 'long',
				day: 'numeric',
				year: 'numeric',
				hour: 'numeric',
				minute: '2-digit'
			})
		});
	}

	if (photo.camera_label) rows.push({ label: 'Camera', value: photo.camera_label });

	const dims = formatDimensions(photo);
	if (dims) rows.push({ label: 'Dimensions', value: dims });

	const size = formatFileSize(photo);
	if (size) rows.push({ label: 'File size', value: size });

	if (photo.mime_type) rows.push({ label: 'Type', value: photo.mime_type });

	return rows;
}
