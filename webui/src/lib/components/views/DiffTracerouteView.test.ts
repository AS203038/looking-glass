import { fromBinary, fromJson, toBinary, type JsonObject } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import DiffTracerouteView from './DiffTracerouteView.svelte';

// The client uses the binary format, so each rtt_ms arrives as a float32.
const traceroute = (hops: JsonObject[]) =>
	fromBinary(
		Pb.TracerouteParsedSchema,
		toBinary(Pb.TracerouteParsedSchema, fromJson(Pb.TracerouteParsedSchema, { hops }))
	);

// Returns the RTT cell of each body row, one span after the other.
function renderRtts(oldHops: JsonObject[], newHops: JsonObject[]): string[] {
	const { container } = render(DiffTracerouteView, {
		oldTp: traceroute(oldHops),
		newTp: traceroute(newHops)
	});
	return Array.from(container.querySelectorAll('tbody tr td:last-child'), (td) =>
		Array.from(td.querySelectorAll('span'), (s) => s.textContent).join(' ')
	);
}

describe('DiffTracerouteView', () => {
	it('shows each RTT as the router printed it', () => {
		const hop1: JsonObject = {
			ttl: 1,
			probes: [{ ip: '192.0.2.1', rttMs: 0.682 }, { rttMs: 0.514 }]
		};
		const rtts = renderRtts(
			[hop1, { ttl: 2, probes: [{ ip: '198.51.100.1', rttMs: 2505.862 }] }],
			[hop1, { ttl: 2, probes: [{ ip: '198.51.100.1', rttMs: 2505.863 }, { rttMs: 0 }] }]
		);
		expect(rtts).toEqual(['0.682 ms 0.514 ms', '2505.862 ms 2505.863 ms *']);
	});
});
