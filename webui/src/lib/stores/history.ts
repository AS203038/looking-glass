import { writable, get } from 'svelte/store';
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

function uint8ToBase64(u8: Uint8Array): string {
	let binary = '';
	const len = u8.byteLength;
	for (let i = 0; i < len; i++) {
		binary += String.fromCharCode(u8[i]);
	}
	return btoa(binary);
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
	return JSON.parse(json, (key, value) => {
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

function loadHistory(): HistoryEntry[] {
	if (typeof localStorage === 'undefined') return [];
	const stored = localStorage.getItem('lg-history');
	if (!stored) return [];
	try {
		return deserializeHistory(stored);
	} catch (e) {
		console.warn('Failed to parse history from localStorage', e);
		return [];
	}
}

export const history = writable<HistoryEntry[]>(loadHistory());

history.subscribe((entries) => {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem('lg-history', serializeHistory(entries));
	} catch (e) {
		console.warn('Failed to save history to localStorage', e);
		// If quota exceeded, we could trim here, but we already limit to 15 items below
	}
});

export function pushHistory(
	command: CommandValue,
	parameter: string,
	routers: Pb.Router[],
	results: Record<string, ExecResult>
) {
	history.update((h) => {
		const resultsArray = Object.values(results);
		if (resultsArray.length > 0 && resultsArray.every((r) => r.cached)) {
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
			id: crypto.randomUUID(),
			timestamp: new Date(),
			command,
			parameter,
			routers,
			results
		};
		// Keep last 15 entries to avoid hitting localStorage 5MB quota
		const next = [entry, ...h];
		if (next.length > 15) next.length = 15;
		return next;
	});
}
