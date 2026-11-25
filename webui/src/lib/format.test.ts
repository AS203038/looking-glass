import { describe, expect, it } from 'vitest';
import { float32 } from './format';

describe('float32', () => {
	it('formats proto float32 values cleanly without IEEE-754 noise', () => {
		expect(float32(0)).toBe('0');
		expect(float32(Math.fround(0.682))).toBe('0.682');
		expect(float32(Math.fround(33.333))).toBe('33.333');
		expect(float32(Math.fround(2505.862))).toBe('2505.862');
	});
});
