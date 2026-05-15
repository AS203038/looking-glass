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

/** Reactive store carrying the current colour mode. */
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

/** Toggles between light and dark mode. */
export function toggleMode() {
	mode.update((m) => (m === 'dark' ? 'light' : 'dark'));
}
