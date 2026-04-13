import { writable } from 'svelte/store';

// Admin auth state
export const isAdmin = writable(false);

// Active alert (print errors, queue paused)
export const activeAlert = writable<{ type: string; message: string } | null>(null);
