import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/svelte';
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

import { _reset as resetRouters } from '$lib/stores/routers';
import { _resetQuery } from '$lib/stores/query';
import RouterPicker from './RouterPicker.svelte';

describe('RouterPicker', () => {
	beforeEach(() => {
		resetRouters();
		_resetQuery();
		getRoutersMock.mockReset();
		window.history.replaceState(null, '', '/');
	});

	it('renders a Retry button on load error and recovers when clicked', async () => {
		getRoutersMock.mockRejectedValueOnce(new Error('rpc unavailable'));
		const { findByRole, findByText } = render(RouterPicker);

		expect(await findByText('Failed to load routers')).toBeTruthy();
		const retryBtn = await findByRole('button', { name: 'Retry' });

		getRoutersMock.mockResolvedValueOnce({
			routers: [
				fromJson(Pb.RouterSchema, {
					id: '1',
					name: 'ams-01',
					location: 'amsterdam',
					bmpActive: true,
					health: { healthy: true }
				}),
				fromJson(Pb.RouterSchema, {
					id: '2',
					name: 'fra-01',
					location: 'frankfurt',
					bmpActive: false,
					health: { healthy: false }
				})
			],
			nextPage: 0
		});

		await fireEvent.click(retryBtn);
		expect(await findByText('ams-01')).toBeTruthy();
		expect(await findByText('BMP')).toBeTruthy();
	});
});
