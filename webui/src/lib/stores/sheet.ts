import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

export type SheetState = 'hidden' | 'peek' | 'half' | 'full';

const STATE_STORAGE_KEY = 'lg-sheet-state';

/** Pixels reserved above the sheet (header + breathing strip). */
export const HEADER_RESERVED_PX = 56 + 16;

const DEFAULT_DOCK_HEIGHT = 120;

function readPersistedState(): SheetState | null {
	if (!browser) return null;
	try {
		const raw = localStorage.getItem(STATE_STORAGE_KEY);
		if (raw === 'half' || raw === 'full' || raw === 'peek') return raw as SheetState;
		return null;
	} catch {
		return null;
	}
}

/** Current sheet state. */
export const sheetState = writable<SheetState>('hidden');

/** The user's preferred state when expanded. */
export const userPreferredState = writable<SheetState>(readPersistedState() ?? 'half');

/** Live dock height in pixels, fed by a ResizeObserver in CommandDock. */
export const dockHeight = writable<number>(DEFAULT_DOCK_HEIGHT);

if (browser) {
	userPreferredState.subscribe((s) => {
		try {
			if (s !== 'hidden') localStorage.setItem(STATE_STORAGE_KEY, s);
		} catch {
			/* localStorage unavailable */
		}
	});
}

/** Cycles state: hidden → peek → half → full → peek. */
export function toggleSheet() {
	sheetState.update((s) => {
		if (s === 'hidden') return get(userPreferredState);
		if (s === 'peek') return 'half';
		if (s === 'half') return 'full';
		return 'peek';
	});
}

/** Sets the sheet state explicitly. */
export function setSheetState(state: SheetState) {
	sheetState.set(state);
	if (state === 'half' || state === 'full') {
		userPreferredState.set(state);
	}
}

/** Sets the sheet state to peek. */
export function peekSheet() {
	sheetState.set('peek');
}

/** Sets the sheet state to hidden. */
export function hideSheet() {
	sheetState.set('hidden');
}

/** Called by run() to expand the sheet to preferred state. */
export function onRunStarted() {
	const pref = get(userPreferredState);
	sheetState.set(pref === 'hidden' || pref === 'peek' ? 'half' : pref);
}

/** Returns the maximum height the sheet can occupy. */
export function maxHeight(): number {
	if (!browser) return 800;
	const dh = get(dockHeight);
	return Math.max(200, window.innerHeight - HEADER_RESERVED_PX - dh);
}
