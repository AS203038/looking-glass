import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { clearToasts, dismissToast, pushToast, toasts } from './toast';

describe('toast store', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		clearToasts();
	});

	afterEach(() => {
		clearToasts();
		vi.useRealTimers();
	});

	it('auto-dismisses after ttl and cancels timer on manual dismiss', () => {
		const id1 = pushToast({ kind: 'info', title: 'First', ttl: 1000 });
		const id2 = pushToast({ kind: 'success', title: 'Second', ttl: 2000 });
		expect(get(toasts)).toHaveLength(2);

		dismissToast(id1);
		expect(get(toasts).map((t) => t.id)).toEqual([id2]);

		vi.advanceTimersByTime(2000);
		expect(get(toasts)).toHaveLength(0);
	});

	it('caps visible toasts at 5 and evicts the oldest', () => {
		for (let i = 1; i <= 7; i++) {
			pushToast({ kind: 'info', title: `Toast ${i}`, ttl: 0 });
		}
		expect(get(toasts).map((t) => t.title)).toEqual([
			'Toast 3',
			'Toast 4',
			'Toast 5',
			'Toast 6',
			'Toast 7'
		]);
	});
});
