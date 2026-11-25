import { writable } from 'svelte/store';
import type { CommandValue, ExecResult } from './query';
import type { Pb } from '$lib/grpc';

export interface HistoryEntry {
	id: string;
	timestamp: Date;
	command: CommandValue;
	parameter: string;
	routers: Pb.Router[];
	results: Record<string, ExecResult>;
}

const STORAGE_KEY = 'lg-history';
const MAX_HISTORY_ENTRIES = 15;
const LARGE_BYTES_THRESHOLD = 262_144;

function generateUUID(): string {
	if (typeof crypto !== 'undefined' && crypto.randomUUID) {
		return crypto.randomUUID();
	}
	return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
		const r = (Math.random() * 16) | 0;
		const v = c === 'x' ? r : (r & 0x3) | 0x8;
		return v.toString(16);
	});
}

/** Encodes a byte slice to base64 in chunks to avoid excessive string allocations. */
function uint8ToBase64(u8: Uint8Array): string {
	const CHUNK = 0x8000;
	const parts: string[] = [];
	for (let i = 0; i < u8.byteLength; i += CHUNK) {
		parts.push(String.fromCharCode(...u8.subarray(i, i + CHUNK)));
	}
	return btoa(parts.join(''));
}

function base64ToUint8(b64: string): Uint8Array {
	const binary = atob(b64);
	const len = binary.length;
	const bytes = new Uint8Array(len);
	for (let i = 0; i < len; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes;
}

function serializeHistory(entries: HistoryEntry[]): string {
	return JSON.stringify(entries, function (key, value) {
		const orig = this[key];
		if (orig instanceof Date) {
			return { __type: 'Date', value: orig.toISOString() };
		}
		if (orig instanceof Uint8Array) {
			return { __type: 'Uint8Array', value: uint8ToBase64(orig) };
		}
		if (typeof orig === 'bigint') {
			return { __type: 'BigInt', value: orig.toString() };
		}
		return value;
	});
}

function deserializeHistory(json: string): HistoryEntry[] {
	return JSON.parse(json, (_key, value) => {
		if (value && typeof value === 'object') {
			if (value.__type === 'Date') {
				return new Date(value.value);
			}
			if (value.__type === 'Uint8Array') {
				return base64ToUint8(value.value);
			}
			if (value.__type === 'BigInt') {
				return BigInt(value.value);
			}
		}
		return value;
	});
}

function stripLargeBytes(entry: HistoryEntry): HistoryEntry {
	const results: Record<string, ExecResult> = {};
	for (const [k, res] of Object.entries(entry.results)) {
		if (res.bytes && res.bytes.byteLength > LARGE_BYTES_THRESHOLD) {
			results[k] = { ...res, bytes: null };
		} else {
			results[k] = res;
		}
	}
	return { ...entry, results };
}

function persistHistory(entries: HistoryEntry[]) {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(STORAGE_KEY, serializeHistory(entries));
		return;
	} catch {
		// Fall through to quota-recovery trimming below.
	}

	let candidate = entries.map((e, idx) => (idx > 0 ? stripLargeBytes(e) : e));
	while (candidate.length > 0) {
		try {
			localStorage.setItem(STORAGE_KEY, serializeHistory(candidate));
			return;
		} catch {
			candidate = candidate.slice(0, -1);
		}
	}
	try {
		localStorage.removeItem(STORAGE_KEY);
	} catch {
		/* localStorage unavailable */
	}
}

function loadHistory(): HistoryEntry[] {
	if (typeof localStorage === 'undefined') return [];
	const stored = localStorage.getItem(STORAGE_KEY);
	if (!stored) return [];
	try {
		const parsed = deserializeHistory(stored);
		return Array.isArray(parsed) ? parsed : [];
	} catch (e) {
		console.warn('Failed to parse history from localStorage', e);
		return [];
	}
}

/** Reactive store of the most recent executed queries. */
export const history = writable<HistoryEntry[]>(loadHistory());

history.subscribe((entries) => {
	persistHistory(entries);
});

/** Records an executed query in history, skipping all-error runs and duplicate cached hits. */
export function pushHistory(
	command: CommandValue,
	parameter: string,
	routers: Pb.Router[],
	results: Record<string, ExecResult>
) {
	const resultsArray = Object.values(results);
	if (resultsArray.length === 0 || resultsArray.every((r) => r.status === 'error')) {
		return;
	}

	history.update((h) => {
		if (resultsArray.every((r) => r.cached)) {
			const isDuplicate = h.some((entry) => {
				if (entry.command !== command || entry.parameter !== parameter) return false;
				if (entry.routers.length !== routers.length) return false;
				if (!entry.routers.every((r, i) => r.id === routers[i].id)) return false;

				return Object.entries(results).every(([rid, res]) => {
					const prevRes = entry.results[rid];
					return (
						prevRes?.timestamp &&
						res?.timestamp &&
						prevRes.timestamp.getTime() === res.timestamp.getTime()
					);
				});
			});

			if (isDuplicate) {
				return h;
			}
		}

		const entry: HistoryEntry = {
			id: generateUUID(),
			timestamp: new Date(),
			command,
			parameter,
			routers,
			results
		};
		const next = [entry, ...h];
		if (next.length > MAX_HISTORY_ENTRIES) next.length = MAX_HISTORY_ENTRIES;
		return next;
	});
}

/** Removes a single history entry by id. */
export function deleteHistoryEntry(id: string) {
	history.update((h) => h.filter((entry) => entry.id !== id));
}

/** Clears all stored query history. */
export function clearHistory() {
	history.set([]);
}
