import { describe, expect, it } from 'vitest';
import { absoluteTime, isoTime, relativeTime } from './time';

describe('time utilities', () => {
	it('formats relative times for immediate, past, and future timestamps', () => {
		const ref = new Date('2026-06-01T12:00:00Z');
		expect(relativeTime(new Date('2026-06-01T11:59:58Z'), ref)).toBe('just now');
		expect(relativeTime(new Date('2026-06-01T11:55:00Z'), ref)).toMatch(/5/);
		expect(relativeTime(new Date('2026-06-01T12:05:00Z'), ref)).toMatch(/5/);
	});

	it('renders ISO and locale-aware absolute timestamps', () => {
		const d = new Date('2026-06-01T12:00:00Z');
		expect(isoTime(d)).toBe('2026-06-01T12:00:00.000Z');
		expect(absoluteTime(d)).toContain('2026');
	});
});
