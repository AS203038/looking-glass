import { browser } from '$app/env';

/** Reports whether the user prefers reduced motion. */
export function prefersReducedMotion(): boolean {
	if (!browser || typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
		return false;
	}
	return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Returns 0 when reduced motion is preferred, or ms otherwise. */
export function transitionDuration(ms: number): number {
	return prefersReducedMotion() ? 0 : ms;
}
