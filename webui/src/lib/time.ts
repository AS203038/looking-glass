import { readable } from 'svelte/store';
import { browser } from '$app/environment';

const TICK_MS = 30_000;

/** A readable store that publishes a fresh `Date` every 30s. */
export const now = readable<Date>(new Date(), (set) => {
	if (!browser) return;
	const id = setInterval(() => set(new Date()), TICK_MS);
	return () => clearInterval(id);
});

const rtf =
	typeof Intl !== 'undefined' && 'RelativeTimeFormat' in Intl
		? new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
		: null;

interface Unit {
	suffix: string;
	intl: Intl.RelativeTimeFormatUnit;
	seconds: number;
}

const UNITS: Unit[] = [
	{ suffix: 'y', intl: 'year', seconds: 60 * 60 * 24 * 365 },
	{ suffix: 'mo', intl: 'month', seconds: 60 * 60 * 24 * 30 },
	{ suffix: 'w', intl: 'week', seconds: 60 * 60 * 24 * 7 },
	{ suffix: 'd', intl: 'day', seconds: 60 * 60 * 24 },
	{ suffix: 'h', intl: 'hour', seconds: 60 * 60 },
	{ suffix: 'm', intl: 'minute', seconds: 60 },
	{ suffix: 's', intl: 'second', seconds: 1 }
];

/** Formats date as a short delta string relative to ref (default now). */
export function relativeTime(date: Date | number, ref: Date | number = Date.now()): string {
	const dateMs = date instanceof Date ? date.getTime() : date;
	const refMs = ref instanceof Date ? ref.getTime() : ref;
	const diffSec = Math.round((dateMs - refMs) / 1000);
	const absSec = Math.abs(diffSec);

	if (absSec < 5) return 'just now';

	for (const u of UNITS) {
		if (absSec >= u.seconds) {
			const value = Math.round(diffSec / u.seconds);
			if (rtf) return rtf.format(value, u.intl);
			const n = Math.abs(value);
			return diffSec < 0 ? `${n}${u.suffix} ago` : `in ${n}${u.suffix}`;
		}
	}
	return 'just now';
}

/** Renders an absolute, locale-aware timestamp suitable for tooltips. */
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

/** Renders an ISO 8601 timestamp. */
export function isoTime(date: Date | number): string {
	const d = date instanceof Date ? date : new Date(date);
	return d.toISOString();
}
