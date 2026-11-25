import { beforeEach, describe, expect, it, vi } from 'vitest';

const mockRawEnv = vi.hoisted(() => ({}) as Record<string, string | undefined>);

vi.mock('$app/env/public', () => ({
	get PUBLIC_PAGE_TITLE() {
		return mockRawEnv.PUBLIC_PAGE_TITLE;
	},
	get PUBLIC_SENTRY_SAMPLE_RATE() {
		return mockRawEnv.PUBLIC_SENTRY_SAMPLE_RATE;
	},
	get PUBLIC_SENTRY_REPLAY() {
		return mockRawEnv.PUBLIC_SENTRY_REPLAY;
	}
}));

import { _resetEnvCache, getEnv, isSafeUrl, parseLinks } from './env';

describe('env utilities', () => {
	beforeEach(() => {
		for (const k of Object.keys(mockRawEnv)) delete mockRawEnv[k];
		_resetEnvCache();
	});

	it('merges defaults and parses boolean replay flag', () => {
		mockRawEnv.PUBLIC_PAGE_TITLE = 'Custom LG';
		mockRawEnv.PUBLIC_SENTRY_SAMPLE_RATE = '0.25';
		mockRawEnv.PUBLIC_SENTRY_REPLAY = 'true';

		const env = getEnv();
		expect(env.PUBLIC_PAGE_TITLE).toBe('Custom LG');
		expect(env.PUBLIC_SENTRY_SAMPLE_RATE).toBe(0.25);
		expect(env.PUBLIC_SENTRY_REPLAY).toBe(true);
	});

	it('validates safe URLs and rejects dangerous schemes', () => {
		expect(isSafeUrl('https://example.com')).toBe(true);
		expect(isSafeUrl('http://localhost:8080/test')).toBe(true);
		expect(isSafeUrl('mailto:noc@example.com')).toBe(true);
		expect(isSafeUrl('/status')).toBe(true);

		expect(isSafeUrl('//evil.example.com')).toBe(false);
		expect(isSafeUrl('javascript:alert(1)')).toBe(false);
		expect(isSafeUrl('data:text/html,<script>')).toBe(false);
		expect(isSafeUrl('')).toBe(false);
	});

	it('filters unsafe links in parseLinks', () => {
		const links = parseLinks(
			'Docs|https://example.com/docs,Bad|javascript:alert(1),Mail|mailto:ops@example.com,Local|/peering'
		);
		expect(links).toEqual([
			{ name: 'Docs', href: 'https://example.com/docs' },
			{ name: 'Mail', href: 'mailto:ops@example.com' },
			{ name: 'Local', href: '/peering' }
		]);
	});
});
