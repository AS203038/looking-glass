import { describe, expect, it } from 'vitest';
import { bgpStateBadgeClass, formatBgpAge } from './bgp';

describe('bgp helpers', () => {
	it('maps BGP states case-insensitively including reason suffixes', () => {
		expect(bgpStateBadgeClass('Established')).toBe('lg-badge-success');
		expect(bgpStateBadgeClass('idle (Admin)')).toBe('lg-badge-warning');
		expect(bgpStateBadgeClass('Connect')).toBe('lg-badge-warning');
		expect(bgpStateBadgeClass('OpenConfirm')).toBe('lg-badge-info');
		expect(bgpStateBadgeClass('down')).toBe('lg-badge-danger');
	});

	it('formats BGP uptime/age including sub-minute durations', () => {
		expect(formatBgpAge(0n)).toBe('—');
		expect(formatBgpAge(42n)).toBe('42s');
		expect(formatBgpAge(125n)).toBe('2m');
		expect(formatBgpAge(3665n)).toBe('1h1m');
		expect(formatBgpAge(90061n)).toBe('1d1h');
	});
});
