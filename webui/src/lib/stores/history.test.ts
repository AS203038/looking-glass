import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { fromJson } from '@bufbuild/protobuf';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import type { ExecResult } from './query';
import { clearHistory, deleteHistoryEntry, history, pushHistory } from './history';

const makeRouter = (id: number, name: string) =>
	fromJson(Pb.RouterSchema, { id: String(id), name, location: 'ams' });

const makeResult = (overrides: Partial<ExecResult> = {}): ExecResult => ({
	routerId: '1',
	routerName: 'rt-1',
	routerLocation: 'ams',
	status: 'done',
	timestamp: new Date('2026-06-01T10:00:00Z'),
	bytes: new Uint8Array([111, 107]),
	error: null,
	queryCommand: 'ping',
	queryParameter: '1.1.1.1',
	resolvedCommand: 'ping',
	parsed: null,
	parserKind: Pb.ParserKind.UNSPECIFIED,
	parseStatus: Pb.ParseStatus.DISABLED,
	cached: false,
	nextPageToken: '',
	loadingMore: false,
	retryAttempt: 0,
	...overrides
});

describe('history store', () => {
	beforeEach(() => {
		localStorage.clear();
		clearHistory();
		vi.restoreAllMocks();
	});

	it('skips all-error runs and deduplicates cached hits', () => {
		const rt = makeRouter(1, 'rt-1');
		pushHistory('ping', '1.1.1.1', [rt], { '1': makeResult({ status: 'error', error: 'boom' }) });
		expect(get(history)).toHaveLength(0);

		const ts = new Date('2026-06-01T10:00:00Z');
		pushHistory('ping', '1.1.1.1', [rt], { '1': makeResult({ cached: true, timestamp: ts }) });
		pushHistory('ping', '1.1.1.1', [rt], { '1': makeResult({ cached: true, timestamp: ts }) });
		expect(get(history)).toHaveLength(1);
	});

	it('recovers when localStorage throws QuotaExceededError', () => {
		const rt = makeRouter(1, 'rt-1');
		const origSetItem = Storage.prototype.setItem;
		let callCount = 0;
		vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (
			this: Storage,
			key: string,
			value: string
		) {
			callCount++;
			if (callCount === 1) {
				throw new DOMException('QuotaExceededError', 'QuotaExceededError');
			}
			return origSetItem.call(this, key, value);
		});

		const huge = new Uint8Array(300_000).fill(65);
		pushHistory('bgp_route', '1.1.1.0/24', [rt], { '1': makeResult({ bytes: huge }) });
		pushHistory('ping', '8.8.8.8', [rt], { '1': makeResult() });
		expect(get(history).length).toBe(2);
	});

	it('supports deleting individual entries and clearing history', () => {
		const rt = makeRouter(1, 'rt-1');
		pushHistory('ping', '1.1.1.1', [rt], { '1': makeResult() });
		pushHistory('ping', '8.8.8.8', [rt], { '1': makeResult({ queryParameter: '8.8.8.8' }) });
		const entries = get(history);
		expect(entries).toHaveLength(2);

		deleteHistoryEntry(entries[0].id);
		expect(get(history)).toHaveLength(1);
		clearHistory();
		expect(get(history)).toHaveLength(0);
	});
});
