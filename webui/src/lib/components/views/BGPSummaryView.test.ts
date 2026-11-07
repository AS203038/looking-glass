import { fromJson } from '@bufbuild/protobuf';
import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import BGPSummaryView from './BGPSummaryView.svelte';

describe('BGPSummaryView', () => {
	it('gives a state with a reason the color of the state', () => {
		const states = [
			'established',
			'idle',
			'idle (removal in progress)',
			'idle(noif)',
			'active',
			'unknown'
		];
		const summary = fromJson(Pb.BGPSummaryParsedSchema, {
			peers: states.map((state, i) => ({ peerIp: `192.0.2.${i + 1}`, state }))
		});
		const { container } = render(BGPSummaryView, { summary });
		const badges = Array.from(container.querySelectorAll('tbody .lg-badge'), (b) => [
			b.textContent!.trim(),
			b.classList.contains('lg-badge-success')
				? 'success'
				: b.classList.contains('lg-badge-warning')
					? 'warning'
					: b.classList.contains('lg-badge-info')
						? 'info'
						: 'danger'
		]);
		expect(badges).toEqual([
			['established', 'success'],
			['idle', 'warning'],
			['idle (removal in progress)', 'warning'],
			['idle(noif)', 'warning'],
			['active', 'info'],
			['unknown', 'danger']
		]);
	});
});
