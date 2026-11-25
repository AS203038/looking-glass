import { writable } from 'svelte/store';

export type ToastKind = 'error' | 'warning' | 'info' | 'success';

export interface Toast {
	id: number;
	kind: ToastKind;
	title: string;
	message?: string;
	ttl: number;
}

const MAX_TOASTS = 5;

let nextId = 1;
const timers = new Map<number, ReturnType<typeof setTimeout>>();

/** Reactive store of currently-visible toasts. */
export const toasts = writable<Toast[]>([]);

function clearTimer(id: number) {
	const timer = timers.get(id);
	if (timer !== undefined) {
		clearTimeout(timer);
		timers.delete(id);
	}
}

/** Adds a toast and schedules its auto-dismissal; returns the new toast id. */
export function pushToast(t: Omit<Toast, 'id' | 'ttl'> & { ttl?: number }): number {
	const id = nextId++;
	const ttl = t.ttl ?? 6000;
	const toast: Toast = { id, ttl, ...t };
	toasts.update((list) => {
		const next = [...list, toast];
		while (next.length > MAX_TOASTS) {
			const evicted = next.shift();
			if (evicted) clearTimer(evicted.id);
		}
		return next;
	});
	if (ttl > 0 && typeof window !== 'undefined') {
		const timer = setTimeout(() => dismissToast(id), ttl);
		timers.set(id, timer);
	}
	return id;
}

/** Removes the toast with the given id and cancels its timer. */
export function dismissToast(id: number) {
	clearTimer(id);
	toasts.update((list) => list.filter((t) => t.id !== id));
}

/** Dismisses all active toasts and clears all timers. */
export function clearToasts() {
	for (const id of Array.from(timers.keys())) {
		clearTimer(id);
	}
	toasts.set([]);
}
