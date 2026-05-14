/**
 * Runtime configuration loader.
 *
 * The Go backend serves `/_app/env.js` containing a single
 * `export const env = { … }` statement injected from `cfg.Web` at boot
 * (see `pkg/http/webui/webui.go`). That endpoint is the same one
 * SvelteKit's `$env/dynamic/public` virtual module imports, so we
 * delegate to it directly — no custom global state, no double source
 * of truth.
 *
 * In `vite dev` (no Go backend), `$env/dynamic/public` returns the
 * empty object — we layer defaults on top so the dev UI remains
 * usable without a backend running.
 */

import { env as rawEnv } from '$env/dynamic/public';

export interface PublicEnv {
	PUBLIC_PAGE_TITLE: string;
	PUBLIC_HEADER_TEXT: string;
	/** Comma-separated `name|href` pairs */
	PUBLIC_HEADER_LINKS: string;
	PUBLIC_HEADER_LOGO: string;
	PUBLIC_FOOTER_TEXT: string;
	PUBLIC_FOOTER_LINKS: string;
	PUBLIC_FOOTER_LOGO: string;
	PUBLIC_GRPC_URL: string;
	PUBLIC_LG_VERSION: string;
	PUBLIC_SENTRY_DSN: string;
	PUBLIC_SENTRY_ENV: string;
	PUBLIC_SENTRY_SAMPLE_RATE: number;
}

const defaults: PublicEnv = {
	PUBLIC_PAGE_TITLE: 'Looking Glass',
	PUBLIC_HEADER_TEXT: 'Looking Glass',
	PUBLIC_HEADER_LINKS: '',
	PUBLIC_HEADER_LOGO: '',
	PUBLIC_FOOTER_TEXT: '',
	PUBLIC_FOOTER_LINKS: '',
	PUBLIC_FOOTER_LOGO: '',
	PUBLIC_GRPC_URL: '',
	PUBLIC_LG_VERSION: 'dev',
	PUBLIC_SENTRY_DSN: '',
	PUBLIC_SENTRY_ENV: 'dev',
	PUBLIC_SENTRY_SAMPLE_RATE: 0
};

let cached: PublicEnv | null = null;

/** Load the runtime env (merged with sensible dev defaults). Cached on first call. */
export function getEnv(): PublicEnv {
	if (cached) return cached;
	const raw = rawEnv as unknown as Record<string, string | number | undefined>;
	cached = {
		...defaults,
		...Object.fromEntries(Object.entries(raw).filter(([, v]) => v !== undefined && v !== ''))
	} as PublicEnv;

	// Numeric coercions (the Go injector emits them as JSON numbers, but env vars
	// from a dev `.env` would arrive as strings).
	cached.PUBLIC_SENTRY_SAMPLE_RATE = Number(cached.PUBLIC_SENTRY_SAMPLE_RATE) || 0;

	// Dev fallback: when no backend is configured, point gRPC at the current
	// origin so a `vite dev` user can still talk to a separately-running server.
	if (!cached.PUBLIC_GRPC_URL && typeof window !== 'undefined') {
		cached.PUBLIC_GRPC_URL = window.location.origin;
	}
	return cached;
}

/** Parse a comma-separated "name|href,name|href" string into a link list. */
export function parseLinks(s: string): { name: string; href: string }[] {
	if (!s) return [];
	return s
		.split(',')
		.map((pair) => {
			const [name, href] = pair.split('|');
			return name && href ? { name: name.trim(), href: href.trim() } : null;
		})
		.filter((x): x is { name: string; href: string } => x !== null);
}
