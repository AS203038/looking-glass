import { describe, expect, it } from 'vitest';
import { computeVirtualWindow } from './virtual';

describe('computeVirtualWindow', () => {
	it('disables virtualization below threshold', () => {
		expect(computeVirtualWindow(50, 0, 400)).toEqual({
			virtualized: false,
			startIdx: 0,
			endIdx: 50,
			padTopPx: 0,
			padBotPx: 0
		});
	});

	it('computes windowed slice and spacer padding above threshold', () => {
		const win = computeVirtualWindow(500, 2800, 280, {
			rowPx: 28,
			threshold: 200,
			overscan: 5
		});
		expect(win.virtualized).toBe(true);
		expect(win.startIdx).toBe(95);
		expect(win.endIdx).toBe(115);
		expect(win.padTopPx).toBe(95 * 28);
		expect(win.padBotPx).toBe((500 - 115) * 28);
	});
});
