import { fromJson, type JsonObject } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import DiffBGPSummaryView from './DiffBGPSummaryView.svelte';

const summary = (peers: JsonObject[]) =>
	fromJson(Pb.BGPSummaryParsedSchema, { localAsn: 64500, routerId: '192.0.2.254', peers });

const peer = (ip: string, family: string, state: string, prefixesReceived: number): JsonObject => ({
	peerIp: ip,
	peerAsn: 64496,
	state,
	prefixesReceived,
	addressFamily: family
});

// Returns the cells of each body row as [diff, peer, state, prefixes], with whitespace collapsed.
function renderRows(oldPeers: JsonObject[], newPeers: JsonObject[]): string[][] {
	const { container } = render(DiffBGPSummaryView, {
		oldSummary: summary(oldPeers),
		newSummary: summary(newPeers)
	});
	return Array.from(container.querySelectorAll('tbody tr'), (tr) =>
		Array.from(tr.querySelectorAll('td'), (td) => td.textContent!.replace(/\s+/g, ' ').trim())
	);
}

describe('DiffBGPSummaryView', () => {
	it('compares each address family of a peer on its own', () => {
		const rows = renderRows(
			[
				peer('192.0.2.1', 'inet.0', 'established', 10),
				peer('192.0.2.1', 'inet6.0', 'established', 20)
			],
			[
				peer('192.0.2.1', 'inet.0', 'established', 10),
				peer('192.0.2.1', 'inet6.0', 'established', 25)
			]
		);
		expect(rows).toEqual([
			['', '192.0.2.1 inet.0 AS64496', 'established', '10'],
			['~', '192.0.2.1 inet6.0 AS64496', 'established', '20 25']
		]);
	});

	it('shows a peer that went down as a change of each family', () => {
		const rows = renderRows(
			[
				peer('192.0.2.1', 'inet.0', 'established', 10),
				peer('192.0.2.1', 'inet6.0', 'established', 20)
			],
			[peer('192.0.2.1', '', 'active', 0)]
		);
		expect(rows).toEqual([
			['~', '192.0.2.1 inet.0 AS64496', 'established active', '10 0'],
			['~', '192.0.2.1 inet6.0 AS64496', 'established active', '20 0']
		]);
	});

	it('shows a peer that came up as a change of each family', () => {
		const rows = renderRows(
			[peer('192.0.2.1', '', 'active', 0)],
			[
				peer('192.0.2.1', 'inet.0', 'established', 10),
				peer('192.0.2.1', 'inet6.0', 'established', 20)
			]
		);
		expect(rows).toEqual([
			['~', '192.0.2.1 inet.0 AS64496', 'active established', '0 10'],
			['~', '192.0.2.1 inet6.0 AS64496', 'active established', '0 20']
		]);
	});

	it('marks each record of an added or removed peer with + or -', () => {
		const rows = renderRows(
			[
				peer('198.51.100.1', 'inet.0', 'established', 30),
				peer('198.51.100.1', 'inet6.0', 'established', 40),
				peer('198.51.100.3', '', 'idle', 0)
			],
			[
				peer('198.51.100.2', 'inet.0', 'established', 50),
				peer('198.51.100.2', 'inet6.0', 'established', 60),
				peer('198.51.100.4', '', 'connect', 0)
			]
		);
		expect(rows).toEqual([
			['-', '198.51.100.1 inet.0 AS64496', 'established', '30'],
			['-', '198.51.100.1 inet6.0 AS64496', 'established', '40'],
			['+', '198.51.100.2 inet.0 AS64496', 'established', '50'],
			['+', '198.51.100.2 inet6.0 AS64496', 'established', '60'],
			['-', '198.51.100.3 AS64496', 'idle', '0'],
			['+', '198.51.100.4 AS64496', 'connect', '0']
		]);
	});
});
