/**
 * Results-sheet state controller.
 *
 * Three states:
 *   - `hidden`   — sheet is not rendered at all. Default before the first
 *                  Execute. Also reachable by the user dismissing the
 *                  sheet entirely (via the handle's `×` affordance).
 *   - `peek`     — only the handle/summary bar is visible above the
 *                  CommandDock. A single tap or drag-up expands.
 *   - `expand`   — sheet grows to occupy a user-controlled portion of
 *                  the viewport. The picker remains visible behind it.
 *
 * Geometry model:
 *   The sheet and dock are siblings inside one sticky-bottom wrapper
 *   (see `+layout.svelte`), so the sheet cannot occlude the dock by
 *   design. But the sheet still has to know how tall the dock is to
 *   cap its own expand height — otherwise a long sheet would *push*
 *   the dock below the viewport. We track the dock's measured height
 *   in `dockHeight`, updated by a `ResizeObserver` inside the dock,
 *   and the sheet's max height becomes
 *     viewport - HEADER_RESERVED_PX - dockHeight.
 *
 * Height policy (decided per-run, not per-render):
 *   - Each `run()` triggers `onRunStarted()` which:
 *       1. Sets state to `expand`.
 *       2. Bumps `fitGeneration` — a counter the sheet component watches
 *          to re-measure result content and choose between "fit to
 *          content" and "snap to fullscreen".
 *   - The sheet measures its own content (`requestAnimationFrame` after
 *     all result cards exist) and calls `applyAutoHeight(measuredPx)`,
 *     which picks the height:
 *       - `measuredPx + chrome <= viewport - top-reserved - dock` AND
 *         the remaining headroom is *generous* (≥ FULLSCREEN_GAP_PX) →
 *         fit exactly to content.
 *       - Otherwise → snap to the maximum (fullscreen-ish, viewport
 *         minus top-reserved minus dock).
 *   - Once the user drags or arrow-keys the handle, `userPreferredHeight`
 *     is set. On subsequent runs we *honour their preference* instead of
 *     recomputing — but the auto-fit still runs once if their preference
 *     happens to undershoot the new content (clamped, never larger).
 *
 * Persistence:
 *   - `userPreferredHeight` persists in localStorage. Resetting state
 *     to "hidden" on reload is deliberate (least-surprise: a fresh page
 *     gets a fresh focus).
 */

import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

export type SheetState = 'hidden' | 'peek' | 'expand';

const HEIGHT_STORAGE_KEY = 'lg-sheet-height';

/** Hard floor — below this the sheet shrinks to peek instead. */
export const MIN_EXPAND_PX = 200;

/**
 * Reserve above the sheet: header (~56) + a 16 px breathing strip so
 * the picker is always at least partially visible behind the sheet.
 * The dock's own height is tracked separately in `dockHeight`.
 */
export const HEADER_RESERVED_PX = 56 + 16;

/**
 * If the space *left over* above an auto-fitted sheet is smaller than
 * this, snap to fullscreen instead. "Almost fullscreen but not quite"
 * looks like a UI bug; better to commit either way.
 *
 * 160 px ≈ enough room for the picker header + the sticky search bar.
 */
const FULLSCREEN_GAP_PX = 160;

/** Sensible fallback dock height for SSR / first paint before the
 *  ResizeObserver fires. ~120 px matches the mobile two-row form. */
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

export const sheetState = writable<SheetState>('hidden');

/**
 * Current effective height in pixels. Modified by:
 *  - the user's drag/keyboard input (sets `userPreferredHeight` too)
 *  - the per-run auto-fit logic
 *  - viewport / dock resize handlers (re-clamp)
 */
export const sheetHeight = writable<number>(readPersistedHeight() ?? 500);

/**
 * The user's *explicit* preferred height. Null means "no preference yet,
 * auto-fit on every run". Once set (via drag/keys), the value sticks.
 */
export const userPreferredHeight = writable<number | null>(readPersistedHeight());

/**
 * Monotonic counter bumped on every Execute. Components watching this
 * know "the results are about to change, please re-fit".
 */
export const fitGeneration = writable<number>(0);

/**
 * Live dock height in pixels, measured by a `ResizeObserver` in
 * `CommandDock.svelte`. Read by the sheet to compute its own max
 * height. The sheet *re-clamps* whenever this changes — so a growing
 * dock (chip strip wraps, soft keyboard opens) instantly shrinks the
 * sheet instead of being hidden behind it.
 */
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

	// When the dock resizes, the available space for the sheet changes.
	// Re-clamp so a tall chip strip can never push the dock off-screen.
	dockHeight.subscribe(() => reclamp());
}

/** Cycle state for click on handle: hidden → peek → expand → peek → expand … */
export function toggleSheet() {
	sheetState.update((s) => {
		if (s === 'hidden') return 'peek';
		if (s === 'peek') return 'expand';
		return 'peek';
	});
}

export function expandSheet() {
	sheetState.set('expand');
}

export function peekSheet() {
	sheetState.set('peek');
}

export function hideSheet() {
	sheetState.set('hidden');
}

/**
 * Called by `run()` in `$lib/stores/query.ts` every time the user
 * submits a query. Pressing Execute is an explicit request to see
 * results, so we unconditionally open the sheet — including from
 * `hidden` (after a previous dismissal) or `peek`. We also bump
 * `fitGeneration` to trigger the next auto-fit pass.
 */
export function onRunStarted() {
	sheetState.set('expand');
	fitGeneration.update((n) => n + 1);
}

/** Maximum height the sheet can occupy (= effectively fullscreen). */
export function maxHeight(): number {
	if (!browser) return 800;
	const dh = get(dockHeight);
	return Math.max(MIN_EXPAND_PX, window.innerHeight - HEADER_RESERVED_PX - dh);
}

/** Clamp a proposed expand height to the sane window. */
export function clampHeight(px: number): number {
	if (!browser) return px;
	const max = maxHeight();
	return Math.min(max, Math.max(MIN_EXPAND_PX, px));
}

/**
 * Set the sheet height as a *user choice* — persists, locks future
 * auto-fits from overriding it. Called from drag and keyboard handlers.
 */
export function setHeightExplicit(px: number) {
	const clamped = clampHeight(px);
	sheetHeight.set(clamped);
	userPreferredHeight.set(clamped);
}

/**
 * Apply an auto-fit decision based on measured content height.
 * Called by the sheet component once it has laid out its result cards.
 *
 *   - If the user has already set an explicit preference, we respect
 *     it and only *grow* if their preference is too small for the new
 *     content (capped at maxHeight).
 *   - Otherwise: fit-to-content unless the remaining gap above would be
 *     awkwardly small, in which case snap to fullscreen.
 */
export function applyAutoHeight(measuredContentPx: number, handlePx: number) {
	if (!browser) return;
	const max = maxHeight();
	const wanted = clampHeight(measuredContentPx + handlePx);

	const pref = get(userPreferredHeight);

	if (pref !== null) {
		// User set a preference. Honour it, but make sure new content isn't
		// stuffed into an under-sized window.
		const target = Math.min(max, Math.max(pref, wanted));
		sheetHeight.set(target);
		return;
	}

	// No user preference yet → "fit to content unless almost-fullscreen".
	const dh = get(dockHeight);
	const gapIfFitted = window.innerHeight - wanted - HEADER_RESERVED_PX - dh;
	if (wanted >= max || gapIfFitted < FULLSCREEN_GAP_PX) {
		sheetHeight.set(max);
	} else {
		sheetHeight.set(wanted);
	}
}

/**
 * Used by viewport- and dock-resize listeners — re-clamp the current
 * height (no `userPreferredHeight` mutation).
 */
export function reclamp() {
	sheetHeight.update((px) => clampHeight(px));
}
