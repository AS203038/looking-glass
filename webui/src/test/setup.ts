import { vi } from 'vitest';

vi.mock('$env/dynamic/public', () => ({
	env: {}
}));

if (typeof globalThis.ResizeObserver === 'undefined') {
	globalThis.ResizeObserver = class ResizeObserver {
		observe() {}
		unobserve() {}
		disconnect() {}
	};
}

if (typeof Element !== 'undefined' && typeof Element.prototype.animate !== 'function') {
	Element.prototype.animate = () =>
		({
			cancel: () => {},
			finish: () => {},
			play: () => {},
			pause: () => {},
			reverse: () => {},
			finished: Promise.resolve(),
			onfinish: null
		}) as unknown as Animation;
}

if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
	window.matchMedia = () =>
		({
			matches: false,
			media: '',
			onchange: null,
			addListener: () => {},
			removeListener: () => {},
			addEventListener: () => {},
			removeEventListener: () => {},
			dispatchEvent: () => false
		}) as MediaQueryList;
}
