import { fromJson } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import TracerouteView from './TracerouteView.svelte';

describe('TracerouteView', () => {
	it('shows the first probe that answered when an earlier probe was lost', () => {
		const tp = fromJson(Pb.TracerouteParsedSchema, {
			target: '203.0.113.10',
			hops: [
				{
					ttl: 1,
					probes: [
						{ ip: '', hostname: '', rttMs: 0 },
						{ ip: '198.51.100.1', hostname: 'core1.example.net', rttMs: 1.5 },
						{ ip: '198.51.100.2', hostname: 'core2.example.net', rttMs: 2.1 }
					]
				},
				{ ttl: 2, probes: [{ rttMs: 0 }, { rttMs: 0 }, { rttMs: 0 }] }
			]
		});
		const { container } = render(TracerouteView, { tp });
		const rows = Array.from(container.querySelectorAll('tbody tr'), (tr) =>
			Array.from(tr.querySelectorAll('td'), (td) => td.textContent!.replace(/\s+/g, ' ').trim())
		);
		expect(rows).toEqual([
			['1', 'core1.example.net', '198.51.100.1', '*1.5 ms2.1 ms'],
			['2', '—', '*', '***']
		]);
	});
});
