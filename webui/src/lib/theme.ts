/**
 * Two-mode theme controller (Catppuccin Latte ⇄ Mocha).
 *
 * The pre-hydration script in `app.html` has already set the correct
 * `.dark` class on <html> before any JS in here runs, so we just have
 * to mirror that into a reactive store and provide a toggle.
 *
 * Persistence:
 *   - localStorage["lg-mode"] = 'light' | 'dark' | null
 *   - `null` means "follow OS preference" — re-evaluated on every load
 *     via the same matchMedia check the bootstrap script uses.
 */

import { writable, type Writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Mode = 'light' | 'dark';
const STORAGE_KEY = 'lg-mode';

function resolveInitial(): Mode {
	if (!browser) return 'dark';
	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored === 'light' || stored === 'dark') return stored;
	} catch {
		/* ignore */
	}
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function apply(mode: Mode) {
	if (!browser) return;
	document.documentElement.classList.toggle('dark', mode === 'dark');
}

/** Reactive mode store. Subscribers fire on every change. */
export const mode: Writable<Mode> = writable<Mode>(resolveInitial());

if (browser) {
	mode.subscribe((m) => {
		apply(m);
		try {
			localStorage.setItem(STORAGE_KEY, m);
		} catch {
			/* localStorage unavailable */
		}
	});
}

export function toggleMode() {
	mode.update((m) => (m === 'dark' ? 'light' : 'dark'));
}
