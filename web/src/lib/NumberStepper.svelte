<script lang="ts">
	import { onDestroy } from 'svelte';

	/**
	 * A labelled number field with explicit −/+ buttons.
	 *
	 * Native <input type="number"> spinners are only a few pixels tall, which
	 * makes nudging a value fiddly with a mouse and impractical on a
	 * touchscreen. These buttons are sized to a real touch target, repeat while
	 * held, and the field still accepts arrow keys.
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

	// Hold-to-repeat timing. The initial pause keeps a single click from
	// registering as two, then it repeats and speeds up so a wide range (font
	// size runs to 400) is reachable without holding for half a minute.
	const INITIAL_DELAY_MS = 350;
	const REPEAT_MS = 60;
	const ACCEL = [
		{ afterTicks: 25, multiplier: 5 },
		{ afterTicks: 10, multiplier: 2 }
	];

	let holdTimer: ReturnType<typeof setTimeout> | null = null;
	let repeatTimer: ReturnType<typeof setInterval> | null = null;
	// Tracked locally during a hold so each tick builds on the last, rather than
	// waiting for the prop to round-trip through the parent.
	let holdValue = 0;
	let ticks = 0;

	function round(v: number): number {
		const f = Math.pow(10, precision);
		return Math.round(v * f) / f;
	}

	function clampValue(v: number): number {
		return Math.max(min, Math.min(max, v));
	}

	function commit(next: number) {
		if (!Number.isFinite(next)) return;
		onchange(round(clampValue(next)));
	}

	function multiplierFor(t: number): number {
		return ACCEL.find(a => t >= a.afterTicks)?.multiplier ?? 1;
	}

	function applyStep(direction: 1 | -1) {
		const next = clampValue(holdValue + direction * step * multiplierFor(ticks));
		if (next === holdValue) {
			// Hit the bound — no point repeating.
			stopHold();
			return;
		}
		holdValue = next;
		commit(next);
	}

	function startHold(direction: 1 | -1) {
		stopHold();
		holdValue = value;
		ticks = 0;
		applyStep(direction);

		holdTimer = setTimeout(() => {
			repeatTimer = setInterval(() => {
				ticks++;
				applyStep(direction);
			}, REPEAT_MS);
		}, INITIAL_DELAY_MS);
	}

	function stopHold() {
		if (holdTimer) { clearTimeout(holdTimer); holdTimer = null; }
		if (repeatTimer) { clearInterval(repeatTimer); repeatTimer = null; }
	}

	// Arrow keys in the field, since type="text" has no native stepper.
	function handleKeydown(e: KeyboardEvent) {
		const direction = e.key === 'ArrowUp' ? 1 : e.key === 'ArrowDown' ? -1 : 0;
		if (!direction) return;
		e.preventDefault();
		commit(value + direction * step * (e.shiftKey ? 10 : 1));
	}

	onDestroy(stopHold);

	const atMin = $derived(value <= min);
	const atMax = $derived(value >= max);
</script>

<svelte:window onpointerup={stopHold} onpointercancel={stopHold} onblur={stopHold} />

<div class="stepper">
	<span class="label">{label}</span>
	<div class="controls">
		<button
			type="button"
			class="step"
			aria-label="Decrease {label}"
			disabled={atMin}
			onpointerdown={(e) => { e.preventDefault(); startHold(-1); }}
			onpointerleave={stopHold}
			oncontextmenu={(e) => e.preventDefault()}
		>&minus;</button>
		<input
			type="text"
			inputmode="decimal"
			value={value}
			aria-label={label}
			onkeydown={handleKeydown}
			onchange={(e) => commit(Number((e.target as HTMLInputElement).value))}
		/>
		<button
			type="button"
			class="step"
			aria-label="Increase {label}"
			disabled={atMax}
			onpointerdown={(e) => { e.preventDefault(); startHold(1); }}
			onpointerleave={stopHold}
			oncontextmenu={(e) => e.preventDefault()}
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
		/* Stops double-tap-to-zoom and text selection eating a rapid hold. */
		touch-action: manipulation;
		user-select: none;
	}

	.step:hover:not(:disabled) {
		color: var(--accent);
	}

	.step:active:not(:disabled) {
		background: var(--accent);
		color: #fff;
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
