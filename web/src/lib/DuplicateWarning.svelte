<script lang="ts">
	import { previewUrl, type DuplicateInfo } from '$lib/api';

	interface Props {
		/** Name of the file being uploaded, shown so the guest knows which one. */
		fileName: string;
		info: DuplicateInfo;
		/** How many files (including this one) are still to be uploaded. */
		remaining?: number;
		/** applyToAll is true when the guest asked for the same choice on the rest. */
		onDecide: (choice: 'upload' | 'skip', applyToAll: boolean) => void;
	}

	let { fileName, info, remaining = 1, onDecide }: Props = $props();

	let applyToAll = $state(false);

	const uploadedAt = $derived(
		info.uploaded_at
			? new Date(info.uploaded_at).toLocaleString(undefined, {
					month: 'short',
					day: 'numeric',
					hour: 'numeric',
					minute: '2-digit'
				})
			: ''
	);
</script>

<div class="backdrop" role="dialog" aria-modal="true" aria-labelledby="dup-title">
	<div class="modal card">
		<h2 id="dup-title">Already uploaded</h2>

		{#if info.photo_id}
			<img class="thumb" src={previewUrl(info.photo_id)} alt="The copy already in this project" />
		{/if}

		<p class="body">
			{#if info.mine}
				You already added <strong>{fileName}</strong> to this project{uploadedAt
					? ` on ${uploadedAt}`
					: ''}.
			{:else}
				<strong>{fileName}</strong> is already in this project{uploadedAt
					? `, added on ${uploadedAt}`
					: ''}.
			{/if}
		</p>
		<p class="hint">Uploading it again gives you a second copy to edit and print.</p>

		{#if remaining > 1}
			<label class="apply-all">
				<input type="checkbox" bind:checked={applyToAll} />
				Do the same for the other {remaining - 1}
				{remaining - 1 === 1 ? 'photo' : 'photos'}
			</label>
		{/if}

		<div class="actions">
			<button class="ghost" onclick={() => onDecide('skip', applyToAll)}>Skip this one</button>
			<button class="primary" onclick={() => onDecide('upload', applyToAll)}>Upload anyway</button>
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.88);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 200;
		padding: 16px;
	}

	.modal {
		max-width: 400px;
		width: 100%;
		padding: 24px;
		text-align: center;
	}

	h2 {
		font-size: 1.15rem;
		font-weight: 600;
		margin-bottom: 12px;
	}

	.thumb {
		width: 100%;
		max-height: 34vh;
		object-fit: contain;
		border-radius: var(--radius-sm);
		background: #000;
		margin-bottom: 14px;
	}

	.body {
		font-size: 0.95rem;
		overflow-wrap: anywhere;
	}

	.hint {
		color: var(--text-muted);
		font-size: 0.85rem;
		margin-top: 8px;
	}

	.apply-all {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		margin-top: 16px;
		color: var(--text-muted);
		font-size: 0.85rem;
		cursor: pointer;
	}

	.apply-all input {
		width: 18px;
		height: 18px;
	}

	.actions {
		display: flex;
		gap: 10px;
		margin-top: 20px;
	}

	.actions button {
		flex: 1;
		padding: 12px 16px;
		font-size: 0.95rem;
	}
</style>
