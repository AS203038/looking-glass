import { beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { fromJson } from '@bufbuild/protobuf';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import { _reset as resetRouters, clearSelection, routers, selectMany } from '$lib/stores/routers';
import { _resetQuery, hasRun, lastRun, results } from '$lib/stores/query';
import { setSheetState, sheetState } from '$lib/stores/sheet';
import ResultsSheet from './ResultsSheet.svelte';

describe('ResultsSheet', () => {
	beforeEach(() => {
		resetRouters();
		_resetQuery();
		const rt = fromJson(Pb.RouterSchema, {
			id: '1',
			name: 'ams-01',
			location: 'ams',
			health: { healthy: true }
		});
		routers.set([rt]);
		selectMany([1n]);
		hasRun.set(true);
		setSheetState('half');
		lastRun.set({
			command: 'ping',
			parameter: '1.1.1.1',
			routerOrder: ['1'],
			routers: [rt]
		});
		results.set({
			'1': {
				routerId: '1',
				routerName: 'ams-01',
				routerLocation: 'ams',
				status: 'done',
				timestamp: new Date('2026-06-01T12:00:00Z'),
				bytes: new Uint8Array([111, 107]),
				error: null,
				queryCommand: 'ping',
				queryParameter: '1.1.1.1',
				resolvedCommand: 'ping',
				parsed: {
					kind: 'ping',
					data: fromJson(Pb.PingStatsSchema, {
						target: '1.1.1.1',
						packetsSent: 5,
						packetsReceived: 5,
						lossPct: 0,
						rttMinMs: 1,
						rttAvgMs: 2,
						rttMaxMs: 3
					})
				},
				parserKind: Pb.ParserKind.BUILTIN,
				parseStatus: Pb.ParseStatus.OK,
				cached: false,
				nextPageToken: '',
				loadingMore: false,
				retryAttempt: 0
			}
		});
	});

	it('keeps displaying results even after routers are deselected', () => {
		clearSelection();
		const { getByText } = render(ResultsSheet);
		expect(getByText('ams-01')).toBeTruthy();
		expect(getByText(/Ping: 1\.1\.1\.1/)).toBeTruthy();
	});

	it('supports ArrowUp/ArrowDown from peek state without nested buttons', async () => {
		const { container, getByRole } = render(ResultsSheet);
		// Ensure no nested buttons exist in the header.
		expect(container.querySelectorAll('button button')).toHaveLength(0);

		const toggleBtn = getByRole('button', { name: /^Results/i });
		setSheetState('peek');
		await fireEvent.keyDown(toggleBtn, { key: 'ArrowUp' });
		expect(get(sheetState)).toBe('half');

		await fireEvent.keyDown(toggleBtn, { key: 'ArrowUp' });
		expect(get(sheetState)).toBe('full');

		await fireEvent.keyDown(toggleBtn, { key: 'ArrowDown' });
		expect(get(sheetState)).toBe('half');
	});
});
