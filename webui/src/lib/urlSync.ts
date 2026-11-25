import { browser } from '$app/env';
import { COMMANDS, type CommandValue } from './stores/query';

export interface UrlQueryState {
	routers: string[];
	cmd: CommandValue | '';
	param: string;
}

const VALID_COMMANDS = new Set<string>(COMMANDS.map((c) => c.value));

/** Parses `?routers=...&cmd=...&param=...` from a query string. */
export function parseUrlState(search: string): UrlQueryState {
	const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
	const rawRouters = params.get('routers') ?? '';
	const routers = rawRouters
		.split(',')
		.map((s) => s.trim())
		.filter(Boolean);
	const rawCmd = (params.get('cmd') ?? '').trim();
	const cmd = VALID_COMMANDS.has(rawCmd) ? (rawCmd as CommandValue) : '';
	const param = params.get('param') ?? '';
	return { routers, cmd, param };
}

/** Builds a shareable URL containing the selected routers, command, and parameter. */
export function buildShareUrl(
	baseUrl: string,
	state: { routerNames: string[]; cmd: CommandValue | ''; param: string }
): string {
	const url = new URL(baseUrl);
	const search = new URLSearchParams(url.search);
	if (state.routerNames.length > 0) {
		search.set('routers', state.routerNames.join(','));
	} else {
		search.delete('routers');
	}
	if (state.cmd) {
		search.set('cmd', state.cmd);
	} else {
		search.delete('cmd');
	}
	if (state.param.trim()) {
		search.set('param', state.param.trim());
	} else {
		search.delete('param');
	}
	const qs = search.toString();
	url.search = qs ? `?${qs}` : '';
	return url.toString();
}

/** Synchronizes query state to `window.location` without creating browser history entries. */
export function syncUrlState(state: {
	routerNames: string[];
	cmd: CommandValue | '';
	param: string;
}): void {
	if (!browser || typeof window === 'undefined') return;
	const next = buildShareUrl(window.location.href, state);
	if (next !== window.location.href) {
		window.history.replaceState(window.history.state, '', next);
	}
}
