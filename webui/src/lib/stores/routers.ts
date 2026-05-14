/**
 * Router catalogue + selection store.
 *
 * Single source of truth for:
 *   - the list of routers loaded from the backend (paginated transparently),
 *   - the user's current selection (multi-select),
 *   - the load lifecycle (idle | loading | ready | error).
 *
 * Selection is stored as a `Set<bigint>` of router IDs (protobuf uint64
 * arrives as bigint via @bufbuild/protobuf) so toggling is O(1) and the
 * order of `routers` can change without orphaning selections.
 *
 * Once the initial load succeeds, a periodic background refresh keeps the
 * router catalogue (and crucially its health badges + last-check
 * timestamps) in sync with the backend, without flashing the loading
 * skeleton or disturbing the user's selection. The refresh is page-
 * visibility aware: it pauses while the tab is hidden and fires
 * immediately when the tab becomes visible again.
 */

import { writable, derived, get, type Readable } from 'svelte/store';
import { LookingGlassClient, type Pb } from '$lib/grpc';
import { pushToast } from '$lib/toast';

type Router = Pb.Router;

export type LoadState = 'idle' | 'loading' | 'ready' | 'error';

export const routers = writable<Router[]>([]);
export const loadState = writable<LoadState>('idle');
export const selectedIds = writable<Set<bigint>>(new Set());

/** Derived: actual Router objects for the current selection (stable order = `routers` order). */
export const selectedRouters: Readable<Router[]> = derived(
	[routers, selectedIds],
	([$routers, $ids]) => $routers.filter((r) => $ids.has(r.id))
);

export function toggleSelection(id: bigint) {
	selectedIds.update((s) => {
		const next = new Set(s);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		return next;
	});
}

export function clearSelection() {
	selectedIds.set(new Set());
}

export function selectMany(ids: bigint[]) {
	selectedIds.update((s) => {
		const next = new Set(s);
		for (const id of ids) next.add(id);
		return next;
	});
}

// ─── Periodic background refresh ────────────────────────────────────────────
//
// The backend updates router health on a 60 s ticker (`pkg/http/grpc/grpc.go`),
// but health doesn't change so often that we need to poll near that cadence.
// 5 minutes keeps the catalogue acceptably fresh for badge/timestamp display
// while staying light on the server — especially for public instances with
// many concurrent viewers.
const REFRESH_INTERVAL_MS = 5 * 60 * 1000;

let loadStarted = false;
let refreshing = false;
let refreshTimer: ReturnType<typeof setInterval> | null = null;
let visibilityHandler: (() => void) | null = null;

/** Page size for paginated GetRouters calls. */
const PAGE_SIZE = 50;

/**
 * Fetch the complete router catalogue, paginating until the server
 * returns `nextPage === 0`. Returns the assembled array; throws on
 * transport failure so callers can decide how to surface the error.
 */
async function fetchAllRouters(): Promise<Router[]> {
	const client = LookingGlassClient();
	const collected: Router[] = [];
	let page = 1;
	while (page !== 0) {
		const res = await client.getRouters({ limit: PAGE_SIZE, pageToken: page });
		collected.push(...res.routers);
		page = res.nextPage ?? 0;
	}
	return collected;
}

/** Idempotent: safe to call from multiple components. */
export async function loadRouters() {
	if (loadStarted) return;
	loadStarted = true;
	loadState.set('loading');
	try {
		// Initial load streams pages into the store so the UI fills as
		// pages arrive — first-paint feedback for slow networks / large
		// catalogues. The background refresh below uses a single atomic
		// swap instead, to avoid mid-refresh flicker.
		const client = LookingGlassClient();
		let page = 1;
		const collected: Router[] = [];
		while (page !== 0) {
			const res = await client.getRouters({ limit: PAGE_SIZE, pageToken: page });
			collected.push(...res.routers);
			page = res.nextPage ?? 0;
			routers.set([...collected]);
		}
		loadState.set('ready');
		startPeriodicRefresh();
	} catch (err) {
		loadState.set('error');
		const message = err instanceof Error ? err.message : String(err);
		pushToast({
			kind: 'error',
			title: 'Could not load routers',
			message
		});
		console.error('loadRouters failed:', err);
	}
}

/**
 * Silently re-fetch the catalogue and atomically swap the `routers`
 * store. Never touches `loadState` (the picker must not flash back to
 * the skeleton), never pushes toasts (a transient background failure
 * shouldn't nag the user — the next interval will try again). Selection
 * is preserved automatically: `selectedIds` is keyed by router ID, and
 * the derived `selectedRouters` store will simply omit any IDs that
 * disappeared from the refreshed catalogue.
 */
export async function refreshRouters(): Promise<void> {
	if (refreshing) return;
	refreshing = true;
	try {
		const next = await fetchAllRouters();
		routers.set(next);
	} catch (err) {
		// Silent on the UI; surfaced only for operators watching devtools.
		console.warn('refreshRouters failed (will retry on next interval):', err);
	} finally {
		refreshing = false;
	}
}

function startPeriodicRefresh() {
	if (typeof window === 'undefined') return;
	if (refreshTimer !== null) return; // already running

	const tick = () => {
		if (typeof document !== 'undefined' && document.hidden) return;
		void refreshRouters();
	};

	refreshTimer = setInterval(tick, REFRESH_INTERVAL_MS);

	// Page Visibility API: pause polling while the tab is hidden (the
	// `tick` guard above) and fire an immediate refresh the moment the
	// tab regains focus so a returning user sees current data without
	// waiting out the rest of the interval.
	if (typeof document !== 'undefined') {
		visibilityHandler = () => {
			if (!document.hidden) void refreshRouters();
		};
		document.addEventListener('visibilitychange', visibilityHandler);
	}
}

/** Stop the background refresh loop (test/HMR cleanup; rarely used in app code). */
export function stopPeriodicRefresh() {
	if (refreshTimer !== null) {
		clearInterval(refreshTimer);
		refreshTimer = null;
	}
	if (visibilityHandler !== null && typeof document !== 'undefined') {
		document.removeEventListener('visibilitychange', visibilityHandler);
		visibilityHandler = null;
	}
}

/** For test/HMR cleanup only — not used in app code. */
export function _reset() {
	stopPeriodicRefresh();
	loadStarted = false;
	refreshing = false;
	routers.set([]);
	selectedIds.set(new Set());
	loadState.set('idle');
}

/** Convenience: read selection synchronously. */
export function currentSelection(): Router[] {
	return get(selectedRouters);
}
