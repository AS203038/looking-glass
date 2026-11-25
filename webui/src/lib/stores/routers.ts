import { writable, derived, get, type Readable } from 'svelte/store';
import { LookingGlassClient, type Pb } from '$lib/grpc';
import { pushToast } from '$lib/toast';

type Router = Pb.Router;

export type LoadState = 'idle' | 'loading' | 'ready' | 'error';

export const routers = writable<Router[]>([]);
export const loadState = writable<LoadState>('idle');
export const loadError = writable<string | null>(null);
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
export function selectMany(ids: Iterable<bigint>) {
	selectedIds.update((s) => {
		const next = new Set(s);
		for (const id of ids) next.add(id);
		return next;
	});
}

/** Removes the given router ids from the selection in a single update. */
export function deselectMany(ids: Iterable<bigint>) {
	selectedIds.update((s) => {
		const next = new Set(s);
		for (const id of ids) next.delete(id);
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

function pruneInvalidSelections(catalogue: Router[]) {
	const healthyById = new Map<bigint, Router>();
	for (const r of catalogue) {
		if (r.health?.healthy === true) healthyById.set(r.id, r);
	}
	const current = get(selectedIds);
	if (current.size === 0) return;
	const dropped: bigint[] = [];
	for (const id of current) {
		if (!healthyById.has(id)) dropped.push(id);
	}
	if (dropped.length === 0) return;
	deselectMany(dropped);
	pushToast({
		kind: 'warning',
		title: 'Selection updated',
		message: `${dropped.length} router${dropped.length === 1 ? ' is' : 's are'} no longer available and ${dropped.length === 1 ? 'was' : 'were'} deselected.`
	});
}

/** Performs the initial router-catalogue load (idempotent while active/ready). */
export async function loadRouters() {
	if (loadStarted) return;
	loadStarted = true;
	loadState.set('loading');
	loadError.set(null);
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
		loadStarted = false;
		loadState.set('error');
		const message = err instanceof Error ? err.message : String(err);
		loadError.set(message);
		pushToast({
			kind: 'error',
			title: 'Could not load routers',
			message
		});
		console.error('loadRouters failed:', err);
	}
}

/** Retries loading the router catalogue after an error. */
export async function retryLoadRouters() {
	loadStarted = false;
	return loadRouters();
}

/** Silently re-fetches the catalogue, swaps the `routers` store atomically, and prunes stale selections. */
export async function refreshRouters(): Promise<void> {
	if (refreshing) return;
	refreshing = true;
	try {
		const next = await fetchAllRouters();
		routers.set(next);
		pruneInvalidSelections(next);
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
	loadError.set(null);
}

/** Returns the current selection synchronously. */
export function currentSelection(): Router[] {
	return get(selectedRouters);
}
