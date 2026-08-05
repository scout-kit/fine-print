<script lang="ts">
	/**
	 * A labelled number field with explicit −/+ buttons.
	 *
	 * Native <input type="number"> spinners are only a few pixels tall, which
	 * makes nudging a value by one step fiddly with a mouse and impractical on a
	 * touchscreen. These buttons are sized to the same touch target the rest of
	 * the admin UI uses.
	 */
	interface Props {
		label: string;
		value: number;
		min?: number;
		max?: number;
		step?: number;
		/** Digits to keep when stepping, so repeated steps can't drift. */
		precision?: number;
		onchange: (value: number) => void;
	}

	let {
		label,
		value,
		min = -Infinity,
		max = Infinity,
		step = 1,
		precision = 0,
		onchange
	}: Props = $props();

	function round(v: number): number {
		const f = Math.pow(10, precision);
		return Math.round(v * f) / f;
	}

	function commit(next: number) {
		if (!Number.isFinite(next)) return;
		onchange(round(Math.max(min, Math.min(max, next))));
	}

	function nudge(direction: 1 | -1) {
		commit(value + direction * step);
	}

	const atMin = $derived(value <= min);
	const atMax = $derived(value >= max);
</script>

<div class="stepper">
	<span class="label">{label}</span>
	<div class="controls">
		<button
			type="button"
			class="step"
			aria-label="Decrease {label}"
			disabled={atMin}
			onclick={() => nudge(-1)}
		>&minus;</button>
		<input
			type="text"
			inputmode="decimal"
			value={value}
			aria-label={label}
			onchange={(e) => commit(Number((e.target as HTMLInputElement).value))}
		/>
		<button
			type="button"
			class="step"
			aria-label="Increase {label}"
			disabled={atMax}
			onclick={() => nudge(1)}
		>+</button>
	</div>
</div>

<style>
	.stepper {
		display: flex;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}

	.label {
		font-size: 0.7rem;
		color: var(--text-muted);
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.controls {
		display: flex;
		align-items: stretch;
		border: 1px solid var(--border);
		border-radius: 4px;
		overflow: hidden;
		min-width: 0;
	}

	.step {
		flex: 0 0 auto;
		width: 34px;
		min-width: 34px;
		min-height: 34px;
		padding: 0;
		border: none;
		border-radius: 0;
		background: var(--bg-elevated);
		color: var(--text);
		font-size: 1rem;
		line-height: 1;
		cursor: pointer;
		/* Stops double-tap-to-zoom eating rapid taps on touchscreens. */
		touch-action: manipulation;
		user-select: none;
	}

	.step:hover:not(:disabled) {
		color: var(--accent);
	}

	.step:disabled {
		opacity: 0.35;
		cursor: default;
	}

	.controls input {
		flex: 1 1 auto;
		min-width: 0;
		width: 100%;
		min-height: 34px;
		padding: 0 2px;
		border: none;
		border-left: 1px solid var(--border);
		border-right: 1px solid var(--border);
		border-radius: 0;
		background: transparent;
		color: var(--text);
		font-size: 0.8rem;
		text-align: center;
	}

	.controls input:focus {
		outline: none;
		background: rgba(74, 158, 255, 0.08);
	}
</style>
