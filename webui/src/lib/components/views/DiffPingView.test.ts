import { fromBinary, fromJson, toBinary, type JsonObject } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import DiffPingView from './DiffPingView.svelte';

// The client uses the binary format, so each RTT field arrives as a float32.
const stats = (json: JsonObject) =>
	fromBinary(Pb.PingStatsSchema, toBinary(Pb.PingStatsSchema, fromJson(Pb.PingStatsSchema, json)));

describe('DiffPingView', () => {
	it('shows each RTT as the router printed it', () => {
		const { container } = render(DiffPingView, {
			oldStats: stats({ rttMinMs: 0.682, rttAvgMs: 1.1, rttMaxMs: 2505.862 }),
			newStats: stats({ rttMinMs: 0.682, rttAvgMs: 1.3, rttMaxMs: 2505.862 })
		});
		const cell = Array.from(container.querySelectorAll('div > span:first-child')).find(
			(s) => s.textContent === 'Min / Avg / Max'
		)!.nextElementSibling!;
		expect(cell.textContent!.replace(/\s+/g, '')).toBe('0.682/1.1➔1.3/2505.862');
	});
});
