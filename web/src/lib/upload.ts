/**
 * Batch photo upload with duplicate confirmation.
 *
 * The server refuses to register a photo the project already holds and says so
 * with a 409 instead, having written nothing. This walks a batch, asks the
 * caller what to do about each such file, and re-sends the ones the guest
 * wants a second copy of.
 */

import { uploadPhoto, isDuplicateUpload, duplicateInfo, type DuplicateInfo } from '$lib/api';

export interface DuplicatePrompt {
	/** Name of the file that is already in the project. */
	fileName: string;
	info: DuplicateInfo;
	/** Files still to be handled, including this one. */
	remaining: number;
}

export type DuplicateChoice = 'upload' | 'skip';

export interface DuplicateDecision {
	choice: DuplicateChoice;
	/** Apply the same choice to every later duplicate in this batch. */
	applyToAll: boolean;
}

export interface BatchResult {
	/** Ids of the photos that were registered, in upload order. */
	ids: number[];
	/** Duplicates the guest chose not to upload again. */
	skipped: number;
	/** Uploads that failed outright. */
	failed: number;
}

export interface BatchOptions {
	/** Called before each file, 1-based. */
	onProgress?: (index: number, total: number) => void;
	/** Asks the guest whether to upload a duplicate anyway. */
	onDuplicate: (prompt: DuplicatePrompt) => Promise<DuplicateDecision>;
}

export async function uploadBatch(
	files: File[],
	projectId: number,
	{ onProgress, onDuplicate }: BatchOptions
): Promise<BatchResult> {
	const result: BatchResult = { ids: [], skipped: 0, failed: 0 };

	// Set once the guest asks for the same answer for the rest of the batch.
	let standingChoice: DuplicateChoice | null = null;

	for (let i = 0; i < files.length; i++) {
		onProgress?.(i + 1, files.length);

		const file = files[i];
		try {
			const res = await uploadPhoto(file, projectId, standingChoice === 'upload');
			result.ids.push(res.id);
		} catch (e) {
			if (!isDuplicateUpload(e)) {
				console.error('Upload failed for file:', file.name, e);
				result.failed++;
				continue;
			}

			let choice = standingChoice;
			if (choice === null) {
				const decision = await onDuplicate({
					fileName: file.name,
					info: duplicateInfo(e),
					remaining: files.length - i
				});
				choice = decision.choice;
				if (decision.applyToAll) standingChoice = decision.choice;
			}

			if (choice === 'skip') {
				result.skipped++;
				continue;
			}

			try {
				const res = await uploadPhoto(file, projectId, true);
				result.ids.push(res.id);
			} catch (retryErr) {
				console.error('Upload failed for file:', file.name, retryErr);
				result.failed++;
			}
		}
	}

	return result;
}
