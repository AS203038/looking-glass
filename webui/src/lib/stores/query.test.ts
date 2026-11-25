import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { fromJson } from '@bufbuild/protobuf';
import { ConnectError, Code } from '@connectrpc/connect';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';

const clientMock = vi.hoisted(() => ({
	ping: vi.fn(),
	traceroute: vi.fn(),
	bGPSummary: vi.fn(),
	bGPRoute: vi.fn(),
	bGPCommunity: vi.fn(),
	bGPLargeCommunity: vi.fn(),
	bGPASPath: vi.fn(),
	bGPPeerRoutes: vi.fn()
}));

vi.mock('$lib/grpc', async () => {
	const actual = await vi.importActual<typeof import('$lib/grpc')>('$lib/grpc');
	return {
		...actual,
		LookingGlassClient: () => clientMock
	};
});

import { _reset as resetRouters, routers, selectMany } from './routers';
import {
	_resetQuery,
	cancelRun,
	formatPeerRoutesParam,
	isRunning,
	loadMorePeerRoutes,
	parsePeerRoutesParam,
	results,
	run
} from './query';

const makeRouter = (id: number, name: string) =>
	fromJson(Pb.RouterSchema, { id: String(id), name, location: 'fra', health: { healthy: true } });

describe('query store', () => {
	beforeEach(() => {
		resetRouters();
		_resetQuery();
		for (const fn of Object.values(clientMock)) fn.mockReset();
		routers.set([makeRouter(1, 'rt-1')]);
		selectMany([1n]);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('parses and formats bgp_peer_routes parameters for both IP and name-only queries', () => {
		expect(parsePeerRoutesParam('192.0.2.1 accepted transit')).toEqual({
			peerIp: '192.0.2.1',
			queryType: 'accepted',
			peerName: 'transit'
		});
		expect(parsePeerRoutesParam('name=ix-peer rejected')).toEqual({
			peerIp: '',
			queryType: 'rejected',
			peerName: 'ix-peer'
		});
		expect(formatPeerRoutesParam('', 'advertised', 'ix-peer')).toBe('name=ix-peer advertised');
		expect(formatPeerRoutesParam('192.0.2.1', 'received', '')).toBe('192.0.2.1 received');
	});

	it('caps ResourceExhausted retries at 5 attempts', async () => {
		vi.useFakeTimers();
		clientMock.ping.mockRejectedValue(new ConnectError('busy', Code.ResourceExhausted));

		const runPromise = run('ping', '1.1.1.1');
		await vi.runAllTimersAsync();
		await runPromise;

		expect(clientMock.ping).toHaveBeenCalledTimes(6);
		expect(get(results)['1'].status).toBe('error');
		expect(get(isRunning)).toBe(false);
	});

	it('cancels an in-flight execution cleanly via cancelRun', async () => {
		clientMock.ping.mockImplementation(
			(_req: unknown, opts?: { signal?: AbortSignal }) =>
				new Promise((_resolve, reject) => {
					opts?.signal?.addEventListener('abort', () => {
						reject(new DOMException('Aborted', 'AbortError'));
					});
				})
		);

		const runPromise = run('ping', '1.1.1.1');
		expect(get(isRunning)).toBe(true);
		cancelRun();
		await runPromise;

		expect(get(isRunning)).toBe(false);
		expect(get(results)['1'].status).toBe('error');
		expect(get(results)['1'].error).toBe('Canceled');
	});

	it('supports bgp_peer_routes pagination via loadMorePeerRoutes', async () => {
		clientMock.bGPPeerRoutes
			.mockResolvedValueOnce(
				fromJson(Pb.BGPPeerRoutesResponseSchema, {
					result: btoa('page1'),
					parseStatus: 'PARSE_STATUS_OK',
					nextPageToken: '50',
					parsed: { paths: [{ prefix: '192.0.2.0/24', nexthop: '10.0.0.1' }] }
				})
			)
			.mockResolvedValueOnce(
				fromJson(Pb.BGPPeerRoutesResponseSchema, {
					result: btoa('page2'),
					parseStatus: 'PARSE_STATUS_OK',
					nextPageToken: '',
					parsed: { paths: [{ prefix: '198.51.100.0/24', nexthop: '10.0.0.1' }] }
				})
			);

		await run('bgp_peer_routes', 'name=ix-peer accepted');
		expect(clientMock.bGPPeerRoutes).toHaveBeenCalledWith(
			expect.objectContaining({
				peerIp: '',
				peerName: 'ix-peer',
				queryType: Pb.PeerRouteQueryType.ACCEPTED
			}),
			expect.anything()
		);
		expect(get(results)['1'].nextPageToken).toBe('50');

		await loadMorePeerRoutes('1');
		const updated = get(results)['1'];
		expect(updated.nextPageToken).toBe('');
		expect(updated.parsed?.kind).toBe('bgp_paths');
		if (updated.parsed?.kind === 'bgp_paths') {
			expect(updated.parsed.data.paths.map((p) => p.prefix)).toEqual([
				'192.0.2.0/24',
				'198.51.100.0/24'
			]);
		}
	});
});
