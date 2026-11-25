import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { fromJson } from '@bufbuild/protobuf';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';

const getRoutersMock = vi.hoisted(() => vi.fn());

vi.mock('$lib/grpc', async () => {
	const actual = await vi.importActual<typeof import('$lib/grpc')>('$lib/grpc');
	return {
		...actual,
		LookingGlassClient: () => ({
			getRouters: getRoutersMock
		})
	};
});

import {
	_reset,
	deselectMany,
	loadError,
	loadRouters,
	loadState,
	refreshRouters,
	retryLoadRouters,
	routers,
	selectedIds,
	selectMany
} from './routers';

const makeRouter = (id: number, name: string, healthy: boolean) =>
	fromJson(Pb.RouterSchema, {
		id: String(id),
		name,
		location: 'fra',
		health: { healthy }
	});

describe('routers store', () => {
	beforeEach(() => {
		_reset();
		getRoutersMock.mockReset();
	});

	it('recovers via retryLoadRouters after an initial load failure', async () => {
		getRoutersMock.mockRejectedValueOnce(new Error('network down'));
		await loadRouters();
		expect(get(loadState)).toBe('error');
		expect(get(loadError)).toBe('network down');

		getRoutersMock.mockResolvedValueOnce({
			routers: [makeRouter(1, 'rt-1', true)],
			nextPage: 0
		});
		await retryLoadRouters();
		expect(get(loadState)).toBe('ready');
		expect(get(loadError)).toBeNull();
		expect(get(routers)).toHaveLength(1);
	});

	it('prunes unhealthy or removed routers during refreshRouters and supports batch deselect', async () => {
		getRoutersMock.mockResolvedValueOnce({
			routers: [
				makeRouter(1, 'rt-1', true),
				makeRouter(2, 'rt-2', true),
				makeRouter(3, 'rt-3', true)
			],
			nextPage: 0
		});
		await loadRouters();
		selectMany([1n, 2n, 3n]);
		expect(get(selectedIds).size).toBe(3);

		deselectMany([3n]);
		expect(Array.from(get(selectedIds))).toEqual([1n, 2n]);

		getRoutersMock.mockResolvedValueOnce({
			routers: [makeRouter(1, 'rt-1', true), makeRouter(2, 'rt-2', false)],
			nextPage: 0
		});
		await refreshRouters();
		expect(Array.from(get(selectedIds))).toEqual([1n]);
	});
});
