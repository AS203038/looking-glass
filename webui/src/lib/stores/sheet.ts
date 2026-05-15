import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

export type SheetState = 'hidden' | 'peek' | 'expand';

const HEIGHT_STORAGE_KEY = 'lg-sheet-height';

/** Minimum expand-mode height in pixels. */
export const MIN_EXPAND_PX = 200;

/** Pixels reserved above the sheet (header + breathing strip). */
export const HEADER_RESERVED_PX = 56 + 16;

const FULLSCREEN_GAP_PX = 160;

const DEFAULT_DOCK_HEIGHT = 120;

function readPersistedHeight(): number | null {
	if (!browser) return null;
	try {
		const raw = localStorage.getItem(HEIGHT_STORAGE_KEY);
		if (!raw) return null;
		const n = Number(raw);
		return Number.isFinite(n) && n >= MIN_EXPAND_PX ? n : null;
	} catch {
		return null;
	}
}

/** Current sheet state. */
export const sheetState = writable<SheetState>('hidden');

/** Current sheet height in pixels. */
export const sheetHeight = writable<number>(readPersistedHeight() ?? 500);

/** The user's explicit preferred height, or null when none has been set. */
export const userPreferredHeight = writable<number | null>(readPersistedHeight());

/** Monotonic counter bumped on every Execute to trigger re-fit. */
export const fitGeneration = writable<number>(0);

/** Live dock height in pixels, fed by a ResizeObserver in CommandDock. */
export const dockHeight = writable<number>(DEFAULT_DOCK_HEIGHT);

if (browser) {
	userPreferredHeight.subscribe((px) => {
		try {
			if (px === null) localStorage.removeItem(HEIGHT_STORAGE_KEY);
			else localStorage.setItem(HEIGHT_STORAGE_KEY, String(Math.round(px)));
		} catch {
			/* localStorage unavailable */
		}
	});

	dockHeight.subscribe(() => reclamp());
}

/** Cycles state: hidden → peek → expand → peek. */
export function toggleSheet() {
	sheetState.update((s) => {
		if (s === 'hidden') return 'peek';
		if (s === 'peek') return 'expand';
		return 'peek';
	});
}

/** Sets the sheet state to expand. */
export function expandSheet() {
	sheetState.set('expand');
}

/** Sets the sheet state to peek. */
export function peekSheet() {
	sheetState.set('peek');
}

/** Sets the sheet state to hidden. */
export function hideSheet() {
	sheetState.set('hidden');
}

/** Called by run() to expand the sheet and bump fitGeneration. */
export function onRunStarted() {
	sheetState.set('expand');
	fitGeneration.update((n) => n + 1);
}

/** Returns the maximum height the sheet can occupy. */
export function maxHeight(): number {
	if (!browser) return 800;
	const dh = get(dockHeight);
	return Math.max(MIN_EXPAND_PX, window.innerHeight - HEADER_RESERVED_PX - dh);
}

/** Clamps a proposed expand height to [MIN_EXPAND_PX, maxHeight()]. */
export function clampHeight(px: number): number {
	if (!browser) return px;
	const max = maxHeight();
	return Math.min(max, Math.max(MIN_EXPAND_PX, px));
}

/** Sets the sheet height as an explicit user preference (persists). */
export function setHeightExplicit(px: number) {
	const clamped = clampHeight(px);
	sheetHeight.set(clamped);
	userPreferredHeight.set(clamped);
}

/** Applies an auto-fit decision based on measured content height. */
export function applyAutoHeight(measuredContentPx: number, handlePx: number) {
	if (!browser) return;
	const max = maxHeight();
	const wanted = clampHeight(measuredContentPx + handlePx);

	const pref = get(userPreferredHeight);

	if (pref !== null) {
		const target = Math.min(max, Math.max(pref, wanted));
		sheetHeight.set(target);
		return;
	}

	const dh = get(dockHeight);
	const gapIfFitted = window.innerHeight - wanted - HEADER_RESERVED_PX - dh;
	if (wanted >= max || gapIfFitted < FULLSCREEN_GAP_PX) {
		sheetHeight.set(max);
	} else {
		sheetHeight.set(wanted);
	}
}

/** Re-clamps the current height to the current bounds. */
export function reclamp() {
	sheetHeight.update((px) => clampHeight(px));
}
