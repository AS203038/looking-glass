import { env as rawEnv } from '$env/dynamic/public';

export interface PublicEnv {
	PUBLIC_PAGE_TITLE: string;
	PUBLIC_HEADER_TEXT: string;
	/** Comma-separated `name|href` pairs. */
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
	PUBLIC_SENTRY_REPLAY: boolean;
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
	PUBLIC_SENTRY_SAMPLE_RATE: 0,
	PUBLIC_SENTRY_REPLAY: false
};

let cached: PublicEnv | null = null;

/** Returns the runtime env merged with defaults; cached on first call. */
export function getEnv(): PublicEnv {
	if (cached) return cached;
	const raw = rawEnv as unknown as Record<string, string | number | boolean | undefined>;
	cached = {
		...defaults,
		...Object.fromEntries(Object.entries(raw).filter(([, v]) => v !== undefined && v !== ''))
	} as PublicEnv;

	cached.PUBLIC_SENTRY_SAMPLE_RATE = Number(cached.PUBLIC_SENTRY_SAMPLE_RATE) || 0;
	const rawReplay = raw.PUBLIC_SENTRY_REPLAY;
	cached.PUBLIC_SENTRY_REPLAY = rawReplay === true || rawReplay === 'true';

	if (!cached.PUBLIC_GRPC_URL && typeof window !== 'undefined') {
		cached.PUBLIC_GRPC_URL = window.location.origin;
	}
	return cached;
}

/** Clears the memoized env cache; for unit tests. */
export function _resetEnvCache(): void {
	cached = null;
}

/** Reports whether href uses a safe link scheme (http, https, mailto, or single-slash relative path). */
export function isSafeUrl(href: string): boolean {
	const trimmed = href.trim();
	if (!trimmed) return false;
	if (trimmed.startsWith('/') && !trimmed.startsWith('//')) return true;
	try {
		const url = new URL(trimmed);
		return url.protocol === 'http:' || url.protocol === 'https:' || url.protocol === 'mailto:';
	} catch {
		return false;
	}
}

/** Parses a comma-separated "name|href,name|href" string into a safe link list. */
export function parseLinks(s: string): { name: string; href: string }[] {
	if (!s) return [];
	return s
		.split(',')
		.map((pair) => {
			const idx = pair.indexOf('|');
			if (idx === -1) return null;
			const name = pair.slice(0, idx).trim();
			const href = pair.slice(idx + 1).trim();
			return name && href && isSafeUrl(href) ? { name, href } : null;
		})
		.filter((x): x is { name: string; href: string } => x !== null);
}
