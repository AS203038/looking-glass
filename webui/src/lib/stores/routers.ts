import { writable, derived, get, type Readable } from 'svelte/store';
import { LookingGlassClient, type Pb } from '$lib/grpc';
import { pushToast } from '$lib/toast';

type Router = Pb.Router;

export type LoadState = 'idle' | 'loading' | 'ready' | 'error';

export const routers = writable<Router[]>([]);
export const loadState = writable<LoadState>('idle');
export const selectedIds = writable<Set<bigint>>(new Set());

/** Derived store carrying the currently-selected router objects. */
export const selectedRouters: Readable<Router[]> = derived(
	[routers, selectedIds],
	([$routers, $ids]) => $routers.filter((r) => $ids.has(r.id))
);

/** Toggles selection of a single router by id. */
export function toggleSelection(id: bigint) {
	selectedIds.update((s) => {
		const next = new Set(s);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		return next;
	});
}

/** Clears the entire selection. */
export function clearSelection() {
	selectedIds.set(new Set());
}

/** Adds the given router ids to the selection. */
export function selectMany(ids: bigint[]) {
	selectedIds.update((s) => {
		const next = new Set(s);
		for (const id of ids) next.add(id);
		return next;
	});
}

const REFRESH_INTERVAL_MS = 5 * 60 * 1000;

let loadStarted = false;
let refreshing = false;
let refreshTimer: ReturnType<typeof setInterval> | null = null;

const PAGE_SIZE = 50;

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

/** Performs the initial router-catalogue load (idempotent). */
export async function loadRouters() {
	if (loadStarted) return;
	loadStarted = true;
	loadState.set('loading');
	try {
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

/** Silently re-fetches the catalogue and swaps the `routers` store atomically. */
export async function refreshRouters(): Promise<void> {
	if (refreshing) return;
	refreshing = true;
	try {
		const next = await fetchAllRouters();
		routers.set(next);
	} catch (err) {
		console.warn('refreshRouters failed (will retry on next interval):', err);
	} finally {
		refreshing = false;
	}
}

function startPeriodicRefresh() {
	if (typeof window === 'undefined') return;
	if (refreshTimer !== null) return;

	const tick = () => {
		if (typeof document !== 'undefined' && document.hidden) return;
		void refreshRouters();
	};

	refreshTimer = setInterval(tick, REFRESH_INTERVAL_MS);
}

/** Stops the background refresh loop. */
export function stopPeriodicRefresh() {
	if (refreshTimer !== null) {
		clearInterval(refreshTimer);
		refreshTimer = null;
	}
}

/** Resets all router state; for test and HMR cleanup. */
export function _reset() {
	stopPeriodicRefresh();
	loadStarted = false;
	refreshing = false;
	routers.set([]);
	selectedIds.set(new Set());
	loadState.set('idle');
}

/** Returns the current selection synchronously. */
export function currentSelection(): Router[] {
	return get(selectedRouters);
}
