import { writable } from 'svelte/store';

export type ToastKind = 'error' | 'warning' | 'info' | 'success';

export interface Toast {
	id: number;
	kind: ToastKind;
	title: string;
	message?: string;
	ttl: number;
}

let nextId = 1;
/** Reactive store of currently-visible toasts. */
export const toasts = writable<Toast[]>([]);

/** Adds a toast and schedules its auto-dismissal; returns the new toast id. */
export function pushToast(t: Omit<Toast, 'id' | 'ttl'> & { ttl?: number }): number {
	const id = nextId++;
	const ttl = t.ttl ?? 6000;
	const toast: Toast = { id, ttl, ...t };
	toasts.update((list) => [...list, toast]);
	if (ttl > 0 && typeof window !== 'undefined') {
		window.setTimeout(() => dismissToast(id), ttl);
	}
	return id;
}

/** Removes the toast with the given id. */
export function dismissToast(id: number) {
	toasts.update((list) => list.filter((t) => t.id !== id));
}
