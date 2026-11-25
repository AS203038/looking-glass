import { describe, expect, it } from 'vitest';
import { buildShareUrl, parseUrlState } from './urlSync';

describe('urlSync', () => {
	it('parses valid commands, router lists, and parameters', () => {
		expect(parseUrlState('?routers=ams-01,fra-01&cmd=bgp_route&param=1.1.1.0/24')).toEqual({
			routers: ['ams-01', 'fra-01'],
			cmd: 'bgp_route',
			param: '1.1.1.0/24'
		});
	});

	it('ignores unknown commands', () => {
		expect(parseUrlState('?cmd=drop_tables&param=x')).toEqual({
			routers: [],
			cmd: '',
			param: 'x'
		});
	});

	it('builds and clears shareable query parameters cleanly', () => {
		const url = buildShareUrl('https://lg.example.net/', {
			routerNames: ['ams-01', 'fra-01'],
			cmd: 'ping',
			param: '192.0.2.1'
		});
		expect(url).toBe('https://lg.example.net/?routers=ams-01%2Cfra-01&cmd=ping&param=192.0.2.1');

		const cleared = buildShareUrl(url, { routerNames: [], cmd: '', param: '' });
		expect(cleared).toBe('https://lg.example.net/');
	});
});
