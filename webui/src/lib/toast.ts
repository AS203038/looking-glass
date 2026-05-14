/**
 * Lightweight toast store. No external dep.
 *
 * Consumers:
 *   - `<Toasts />` renders the visible stack
 *   - `pushToast({ kind, title, message })` adds one
 *   - Each toast auto-dismisses after `ttl` ms (default 6 s)
 */

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
export const toasts = writable<Toast[]>([]);

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

export function dismissToast(id: number) {
	toasts.update((list) => list.filter((t) => t.id !== id));
}
