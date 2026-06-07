<script lang="ts">
	import { fly } from 'svelte/transition';
	import {
		sheetState,
		dockHeight,
		toggleSheet,
		setSheetState,
		hideSheet,
		maxHeight,
		HEADER_RESERVED_PX,
		type SheetState
	} from '$lib/stores/sheet';
	import { results, hasRun } from '$lib/stores/query';
	import { selectedRouters } from '$lib/stores/routers';
	import type { Pb } from '$lib/grpc';
	import type { ExecResult } from '$lib/stores/query';
	import ResultCard from './ResultCard.svelte';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import X from '@lucide/svelte/icons/x';

	let sheetCurrent: SheetState = $state('hidden');
	let dockPx = $state(0);
	let res = $state<Record<string, ExecResult>>({});
	let selected = $state<Pb.Router[]>([]);
	let ran = $state(false);

	sheetState.subscribe((s) => (sheetCurrent = s));
	dockHeight.subscribe((d) => (dockPx = d));
	results.subscribe((v) => (res = v));
	selectedRouters.subscribe((v) => (selected = v));
	hasRun.subscribe((v) => (ran = v));

	const maxHeightCss = $derived(
		`calc(100dvh - ${HEADER_RESERVED_PX}px - ${Math.max(0, dockPx)}px)`
	);

	const ordered = $derived.by(() => {
		const out: ExecResult[] = [];
		for (const r of selected) {
			const key = r.id.toString();
			if (res[key]) out.push(res[key]);
		}
		return out;
	});

	const counts = $derived.by(() => {
		const c = { running: 0, done: 0, error: 0 };
		for (const r of ordered) {
			if (r.status === 'pending' || r.status === 'running') c.running++;
			else if (r.status === 'done') c.done++;
			else if (r.status === 'error') c.error++;
		}
		return c;
	});

	let bodyEl = $state<HTMLDivElement | null>(null);
	let handleEl = $state<HTMLDivElement | null>(null);

	let dragHeightPx = $state(0);

	const maxPx = $derived(maxHeight());
	const halfPx = $derived(Math.max(200, maxPx / 2));
	const peekPx = $derived(handleEl ? handleEl.offsetHeight : 44);

	const currentHeightPx = $derived.by(() => {
		if (dragging) return dragHeightPx;
		if (sheetCurrent === 'full') return maxPx;
		if (sheetCurrent === 'half') return halfPx;
		return peekPx;
	});

	const DRAG_THRESHOLD_PX = 4;
	let dragging = $state(false);
	let pointerActive = false;
	let wasDrag = false;
	let dragStartY = 0;
	let dragStartHeight = 0;

	function onHandlePointerDown(e: PointerEvent) {
		if (e.button !== undefined && e.button !== 0) return;
		pointerActive = true;
		wasDrag = false;
		dragStartY = e.clientY;
		dragStartHeight = currentHeightPx;
		try {
			(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		} catch {
			/* no-op */
		}
	}

	function onHandlePointerMove(e: PointerEvent) {
		if (!pointerActive) return;
		const delta = dragStartY - e.clientY;

		if (!dragging) {
			if (Math.abs(delta) < DRAG_THRESHOLD_PX) return;
			dragging = true;
			wasDrag = true;
			dragHeightPx = dragStartHeight;
		}

		dragHeightPx = Math.max(peekPx, Math.min(maxPx, dragStartHeight + delta));
	}

	function endDrag(e: PointerEvent) {
		if (!pointerActive) return;
		pointerActive = false;
		if (dragging) {
			dragging = false;
			const dPeek = Math.abs(dragHeightPx - peekPx);
			const dHalf = Math.abs(dragHeightPx - halfPx);
			const dFull = Math.abs(dragHeightPx - maxPx);

			const min = Math.min(dPeek, dHalf, dFull);
			if (min === dFull) setSheetState('full');
			else if (min === dHalf) setSheetState('half');
			else setSheetState('peek');
		}
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* no-op */
		}
	}

	function onHandleClick() {
		if (wasDrag) {
			wasDrag = false;
			return;
		}
		toggleSheet();
	}

	function onHandleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			toggleSheet();
			return;
		}
		if (sheetCurrent === 'half' || sheetCurrent === 'full') {
			if (e.key === 'ArrowUp') {
				e.preventDefault();
				setSheetState(sheetCurrent === 'half' ? 'full' : 'full');
			} else if (e.key === 'ArrowDown') {
				e.preventDefault();
				setSheetState(sheetCurrent === 'full' ? 'half' : 'peek');
			}
		}
	}
</script>

{#if sheetCurrent !== 'hidden' && ran}
	<aside
		role="region"
		aria-label="Results"
		class="flex flex-col border-t backdrop-blur-md transition-[height]"
		style="background-color: color-mix(in oklab, var(--color-bg-mantle) 96%, transparent);
		       border-color: var(--color-border);
		       height: {currentHeightPx}px;
		       transition-duration: {dragging ? '0ms' : '200ms'};
		       transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
		       max-height: {maxHeightCss};"
		in:fly={{ y: 16, duration: 160 }}
	>
		<div
			role="button"
			tabindex="0"
			bind:this={handleEl}
			class="relative flex h-11 shrink-0 cursor-row-resize items-center gap-3 border-b px-4 select-none sm:h-9 sm:px-6"
			style="border-color: var(--color-border); touch-action: none; -webkit-user-select: none; -webkit-touch-callout: none;"
			aria-expanded={sheetCurrent === 'half' || sheetCurrent === 'full'}
			aria-controls="lg-results-body"
			onpointerdown={onHandlePointerDown}
			onpointermove={onHandlePointerMove}
			onpointerup={endDrag}
			onpointercancel={endDrag}
			oncontextmenu={(e) => e.preventDefault()}
			onkeydown={onHandleKeyDown}
			onclick={onHandleClick}
			title={sheetCurrent === 'half' || sheetCurrent === 'full'
				? 'Drag to resize. Click to collapse.'
				: 'Click to expand results. Drag to resize.'}
		>
			<span
				class="pointer-events-none absolute top-1.5 left-1/2 h-1.5 w-12 -translate-x-1/2 rounded-full sm:top-1 sm:h-1 sm:w-10"
				style="background-color: var(--color-overlay); opacity: 0.5;"
				aria-hidden="true"
			></span>

			<span class="flex items-center gap-2 text-xs">
				{#if sheetCurrent === 'half' || sheetCurrent === 'full'}
					<ChevronDown size={14} class="opacity-60" />
				{:else}
					<ChevronUp size={14} class="opacity-60" />
				{/if}
				<span class="font-semibold tracking-wide uppercase">Results</span>
				<span style="color: var(--color-fg-muted);">
					· {ordered.length} router{ordered.length === 1 ? '' : 's'}
				</span>
			</span>

			<span class="ml-auto flex items-center gap-1.5" aria-live="polite">
				{#if counts.running > 0}
					<span class="lg-badge lg-badge-info">{counts.running} running</span>
				{/if}
				{#if counts.done > 0}
					<span class="lg-badge lg-badge-success">{counts.done} done</span>
				{/if}
				{#if counts.error > 0}
					<span class="lg-badge lg-badge-danger">{counts.error} failed</span>
				{/if}
			</span>

			<button
				type="button"
				class="lg-btn lg-btn-ghost h-8 w-8 shrink-0 !p-0 sm:h-7 sm:w-7"
				aria-label="Dismiss results panel"
				title="Dismiss"
				onpointerdown={(e) => e.stopPropagation()}
				onclick={(e) => {
					e.stopPropagation();
					hideSheet();
				}}
			>
				<X size={14} />
			</button>
		</div>

		{#if sheetCurrent === 'half' || sheetCurrent === 'full' || dragging}
			<div
				id="lg-results-body"
				class="flex flex-1 flex-col overflow-y-auto px-4 py-4 sm:px-6"
				style="overscroll-behavior: contain; --sheet-body-height: {currentHeightPx -
					(handleEl ? handleEl.offsetHeight : 44)}px;"
				bind:this={bodyEl}
			>
				<div
					class="grid w-full flex-1 items-start gap-3"
					style="grid-template-columns: repeat(auto-fit, minmax(min(100%, 40rem), 1fr));"
				>
					{#each ordered as r (r.routerId)}
						<ResultCard result={r} />
					{/each}
				</div>
			</div>
		{/if}
	</aside>
{/if}
