/**
 * Time helpers.
 *
 * - `now` is a Svelte `readable` store that ticks every 30 s. Bind to
 *   it (`$now`) anywhere you render a relative timestamp and the value
 *   will refresh on its own without per-component setInterval bookkeeping.
 * - `relativeTime` formats a `Date` (or anything coercible) as a short
 *   human-readable delta relative to a reference time (default = now).
 *   We use `Intl.RelativeTimeFormat` so the locale of the user's browser
 *   is honoured automatically.
 */

import { readable } from 'svelte/store';
import { browser } from '$app/environment';

const TICK_MS = 30_000;

export const now = readable<Date>(new Date(), (set) => {
	if (!browser) return;
	const id = setInterval(() => set(new Date()), TICK_MS);
	return () => clearInterval(id);
});

// Module-singleton formatter — instantiating Intl.RelativeTimeFormat on
// every call is surprisingly expensive at 60 fps; reuse one instance.
const rtf =
	typeof Intl !== 'undefined' && 'RelativeTimeFormat' in Intl
		? new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
		: null;

interface Unit {
	suffix: string;
	intl: Intl.RelativeTimeFormatUnit;
	seconds: number;
}

// Ordered from largest to smallest. We pick the *largest* unit such that
// `delta >= unit.seconds`; that gives "2 years ago" rather than the noisy
// "730 days ago". `just now` is special-cased below the smallest unit.
const UNITS: Unit[] = [
	{ suffix: 'y', intl: 'year', seconds: 60 * 60 * 24 * 365 },
	{ suffix: 'mo', intl: 'month', seconds: 60 * 60 * 24 * 30 },
	{ suffix: 'w', intl: 'week', seconds: 60 * 60 * 24 * 7 },
	{ suffix: 'd', intl: 'day', seconds: 60 * 60 * 24 },
	{ suffix: 'h', intl: 'hour', seconds: 60 * 60 },
	{ suffix: 'm', intl: 'minute', seconds: 60 },
	{ suffix: 's', intl: 'second', seconds: 1 }
];

/**
 * Format `date` as a short delta string relative to `ref`.
 *
 * Examples (with `ref = date + delta`):
 *   - delta <  5 s  → "just now"
 *   - delta < 60 s  → "30s ago"
 *   - delta <  1 h  → "5m ago"
 *   - delta <  1 d  → "2h ago"
 *   - delta < 1 w   → "3d ago"
 *   - etc.
 *
 * Negative deltas (date in future) are formatted as "in 5m".
 */
export function relativeTime(date: Date | number, ref: Date | number = Date.now()): string {
	const dateMs = date instanceof Date ? date.getTime() : date;
	const refMs = ref instanceof Date ? ref.getTime() : ref;
	const diffSec = Math.round((dateMs - refMs) / 1000); // negative = past
	const absSec = Math.abs(diffSec);

	if (absSec < 5) return 'just now';

	for (const u of UNITS) {
		if (absSec >= u.seconds) {
			const value = Math.round(diffSec / u.seconds);
			if (rtf) return rtf.format(value, u.intl);
			// Fallback if Intl.RelativeTimeFormat is missing.
			const n = Math.abs(value);
			return diffSec < 0 ? `${n}${u.suffix} ago` : `in ${n}${u.suffix}`;
		}
	}
	return 'just now';
}

/** Absolute formatter — for tooltips. Always shows the same locale-default. */
export function absoluteTime(date: Date | number): string {
	const d = date instanceof Date ? date : new Date(date);
	return d.toLocaleString(undefined, {
		year: 'numeric',
		month: 'short',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
}

/** ISO 8601 (machine-readable, for `<time datetime="…">`). */
export function isoTime(date: Date | number): string {
	const d = date instanceof Date ? date : new Date(date);
	return d.toISOString();
}
