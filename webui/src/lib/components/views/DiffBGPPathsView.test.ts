import { fromJson } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import DiffBGPPathsView from './DiffBGPPathsView.svelte';

describe('DiffBGPPathsView', () => {
	it('displays Prefix, Peer, and Communities columns and detects community/best-path diffs', () => {
		const oldPaths = fromJson(Pb.BGPPathsSchema, {
			paths: [
				{
					prefix: '203.0.113.0/24',
					nexthop: '192.0.2.1',
					peerAsn: 64496,
					peerIp: '192.0.2.1',
					best: true,
					communities: ['64496:1']
				}
			]
		});
		const newPaths = fromJson(Pb.BGPPathsSchema, {
			paths: [
				{
					prefix: '203.0.113.0/24',
					nexthop: '192.0.2.1',
					peerAsn: 64496,
					peerIp: '192.0.2.1',
					best: false,
					communities: ['64496:2']
				}
			]
		});

		const { container } = render(DiffBGPPathsView, { oldPaths, newPaths });
		const headers = Array.from(container.querySelectorAll('thead th'), (th) =>
			th.textContent!.trim()
		);
		expect(headers).toContain('Prefix');
		expect(headers).toContain('Peer');
		expect(headers).toContain('Communities');

		const rowText = container.querySelector('tbody tr')!.textContent!.replace(/\s+/g, ' ').trim();
		expect(rowText).toContain('~');
		expect(rowText).toContain('203.0.113.0/24');
		expect(rowText).toContain('AS64496 192.0.2.1');
		expect(rowText).toContain('64496:1');
		expect(rowText).toContain('64496:2');
	});
});
