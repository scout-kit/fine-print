import { writable, type Readable } from 'svelte/store';

export interface SSEEvent {
	type: string;
	data: unknown;
	timestamp: string;
}

export interface SSEState {
	connected: boolean;
	lastEvent: SSEEvent | null;
	alert: SSEEvent | null;
}

export interface SSEConnection {
	state: Readable<SSEState>;
	close: () => void;
	clearAlert: () => void;
}

export function createSSE(url: string): SSEConnection {
	const state = writable<SSEState>({
		connected: false,
		lastEvent: null,
		alert: null
	});

	const source = new EventSource(url);

	source.onopen = () => {
		state.update(s => ({ ...s, connected: true }));
	};

	source.onerror = () => {
		state.update(s => ({ ...s, connected: false }));
	};

	// Listen for all event types
	for (const type of [
		'connected', 'photo_status', 'print_status',
		'print_error', 'queue_paused', 'queue_resumed', 'new_photo',
		'settings_changed', 'restarting',
		'printer_disconnected', 'printer_reconnected'
	]) {
		source.addEventListener(type, (e: MessageEvent) => {
			try {
				const event: SSEEvent = JSON.parse(e.data);
				state.update(s => {
					const updated: SSEState = { ...s, lastEvent: event };
					if (
						type === 'print_error' ||
						type === 'queue_paused' ||
						type === 'printer_disconnected'
					) {
						updated.alert = event;
					}
					return updated;
				});
			} catch {
				// Ignore parse errors
			}
		});
	}

	return {
		state,
		close: () => source.close(),
		clearAlert: () => state.update(s => ({ ...s, alert: null }))
	};
}
