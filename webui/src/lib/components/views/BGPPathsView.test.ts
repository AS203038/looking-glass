import { fromJson } from '@bufbuild/protobuf';
import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import BGPPathsView from './BGPPathsView.svelte';

describe('BGPPathsView', () => {
	it('renders duplicate communities without key collisions and shows origin and age', () => {
		const paths = fromJson(Pb.BGPPathsSchema, {
			paths: [
				{
					prefix: '203.0.113.0/24',
					nexthop: '192.0.2.1',
					asPath: [64496],
					origin: 'igp',
					ageSeconds: '90',
					communities: ['64496:100', '64496:100'],
					largeCommunities: ['64496:1:2', '64496:1:2'],
					best: true
				}
			]
		});
		const { container } = render(BGPPathsView, { paths });
		const text = container.textContent!.replace(/\s+/g, ' ');
		expect(text).toContain('203.0.113.0/24');
		expect(text).toContain('64496:100');
		expect(text).toContain('64496:1:2');
		expect(text).toContain('1m');
	});

	it('renders Load more routes button when nextPageToken is set', async () => {
		const onLoadMore = vi.fn();
		const paths = fromJson(Pb.BGPPathsSchema, {
			paths: [{ prefix: '203.0.113.0/24', nexthop: '192.0.2.1' }]
		});
		const { getByRole } = render(BGPPathsView, {
			paths,
			nextPageToken: '50',
			onLoadMore
		});
		await fireEvent.click(getByRole('button', { name: 'Load more routes' }));
		expect(onLoadMore).toHaveBeenCalledOnce();
	});
});
