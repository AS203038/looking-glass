<script lang="ts">
	/**
	 * Results sheet — anchored above the CommandDock, three-state model
	 * (hidden / peek / expand) with resize-by-drag AND per-run auto-fit.
	 *
	 * Auto-fit policy (see `$lib/stores/sheet.ts` for the full spec):
	 *   - Each `run()` bumps a `fitGeneration` counter. We watch it and,
	 *     once the result cards have laid out (one rAF tick later), we
	 *     measure the natural content height and decide:
	 *       * if it fits *with generous headroom* above → grow exactly
	 *         to content.
	 *       * if it would leave only an awkward strip above (< 160 px)
	 *         → snap to fullscreen instead.
	 *   - The first time the user drags or arrow-keys the handle, we
	 *     record their preference and stop auto-fitting (but still
	 *     ensure new content fits — never shrink their window).
	 *
	 * Drag/click disambiguation:
	 *   The handle is BOTH a click target (cycle state) and a drag
	 *   handle (resize). Without care, releasing a real drag triggers a
	 *   synthetic click that immediately undoes the drag. Solved with a
	 *   `DRAG_THRESHOLD_PX` movement test: gestures below the threshold
	 *   stay clicks; above it, we mark `wasDrag` and the click handler
	 *   no-ops on release.
	 */
	import { onMount, tick } from 'svelte';
	import { fly } from 'svelte/transition';
	import {
		sheetState,
		sheetHeight,
		fitGeneration,
		toggleSheet,
		peekSheet,
		expandSheet,
		hideSheet,
		setHeightExplicit,
		applyAutoHeight,
		reclamp,
		clampHeight,
		MIN_EXPAND_PX,
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

	// Local mirrors of the stores. (`state` is reserved by Svelte's
	// template compiler as the `$state` rune prefix; we use `sheetCurrent`
	// instead.)
	let sheetCurrent: SheetState = $state('hidden');
	let heightPx = $state(0);
	let res = $state<Record<string, ExecResult>>({});
	let selected = $state<Pb.Router[]>([]);
	let ran = $state(false);
	let gen = $state(0);

	sheetState.subscribe((s) => (sheetCurrent = s));
	sheetHeight.subscribe((h) => (heightPx = h));
	results.subscribe((v) => (res = v));
	selectedRouters.subscribe((v) => (selected = v));
	hasRun.subscribe((v) => (ran = v));
	fitGeneration.subscribe((n) => (gen = n));

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

	// ---- Auto-fit on each new run -----------------------------------------
	// `gen` ticks for every Execute. We watch it and, on the next frame
	// (when result cards have rendered their pending/running shells),
	// measure their bounding-box and ask the store to apply the policy.
	//
	// We also schedule a *second* measurement after every result settles
	// from `running` to `done`/`error`, because that's typically when the
	// height changes most — the card grows from a spinner to a code block.
	// We only do this while the user has *no preference* (the policy
	// itself bails out otherwise), so it can never fight a manual resize.
	let bodyEl: HTMLDivElement | null = $state(null);
	const HANDLE_PX = 36;

	function autoFit() {
		if (!bodyEl) return;
		const measured = bodyEl.scrollHeight;
		applyAutoHeight(measured, HANDLE_PX);
	}

	// Initial fit on every new run.
	$effect(() => {
		// Track gen so the effect re-runs on every Execute.
		void gen;
		if (!ran || sheetCurrent !== 'expand') return;
		// rAF defers until the result-card shells are in layout.
		requestAnimationFrame(() => {
			tick().then(autoFit);
		});
	});

	// Re-fit when status counts change (a result finished/errored and
	// likely caused content to grow). Cheap when user has a preference —
	// the store's policy no-ops in that case.
	$effect(() => {
		void counts.done;
		void counts.error;
		void counts.running;
		if (!ran || sheetCurrent !== 'expand') return;
		requestAnimationFrame(autoFit);
	});

	// ---- Drag/click logic --------------------------------------------------
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
		dragStartHeight = heightPx;
		try {
			(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		} catch {
			/* pointer capture may be unavailable */
		}
	}

	function onHandlePointerMove(e: PointerEvent) {
		if (!pointerActive) return;
		const delta = dragStartY - e.clientY;

		if (!dragging) {
			if (Math.abs(delta) < DRAG_THRESHOLD_PX) return;
			dragging = true;
			wasDrag = true;
			if (sheetCurrent === 'peek') expandSheet();
		}

		const next = clampHeight(dragStartHeight + delta);
		// Drag-resize is an explicit user preference: locks future
		// auto-fits from clobbering it.
		setHeightExplicit(next);

		// Drag well below the floor → snap to peek.
		if (dragStartHeight + delta < MIN_EXPAND_PX - 60) {
			peekSheet();
			endDrag(e);
		}
	}

	function endDrag(e: PointerEvent) {
		if (!pointerActive) return;
		pointerActive = false;
		dragging = false;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* pointer already released */
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
		if (sheetCurrent === 'expand') {
			if (e.key === 'ArrowUp') {
				e.preventDefault();
				setHeightExplicit(heightPx + 40);
			} else if (e.key === 'ArrowDown') {
				e.preventDefault();
				const next = heightPx - 40;
				if (next < MIN_EXPAND_PX) peekSheet();
				else setHeightExplicit(next);
			}
		}
	}

	onMount(() => {
		if (typeof window === 'undefined') return;
		const onResize = () => reclamp();
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
	});
</script>

{#if sheetCurrent !== 'hidden' && ran}
	<aside
		role="region"
		aria-label="Results"
		class="sticky bottom-0 z-25 flex flex-col border-t backdrop-blur-md"
		style="background-color: color-mix(in oklab, var(--color-bg-mantle) 96%, transparent);
		       border-color: var(--color-border);
		       height: {sheetCurrent === 'expand' ? `${heightPx}px` : 'auto'};
		       max-height: calc(100dvh - 5rem);"
		in:fly={{ y: 16, duration: 160 }}
	>
		<!-- ── Handle / summary bar ─────────────────────────────────────── -->
		<div
			role="button"
			tabindex="0"
			class="flex h-9 shrink-0 cursor-row-resize items-center gap-3 border-b px-4 select-none sm:px-6"
			style="border-color: var(--color-border); touch-action: none;"
			aria-expanded={sheetCurrent === 'expand'}
			aria-controls="lg-results-body"
			onpointerdown={onHandlePointerDown}
			onpointermove={onHandlePointerMove}
			onpointerup={endDrag}
			onpointercancel={endDrag}
			onkeydown={onHandleKeyDown}
			onclick={onHandleClick}
			title={sheetCurrent === 'expand'
				? 'Drag to resize. Click to collapse.'
				: 'Click to expand results. Drag to resize.'}
		>
			<span
				class="absolute left-1/2 h-1 w-10 -translate-x-1/2 rounded-full"
				style="background-color: var(--color-overlay); opacity: 0.5;"
				aria-hidden="true"
			></span>

			<span class="flex items-center gap-2 text-xs">
				{#if sheetCurrent === 'expand'}
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
				class="lg-btn lg-btn-ghost h-7 w-7 !p-0"
				aria-label="Dismiss results panel"
				title="Dismiss"
				onclick={(e) => {
					e.stopPropagation();
					hideSheet();
				}}
			>
				<X size={14} />
			</button>
		</div>

		<!-- ── Body (only when expanded) ────────────────────────────────── -->
		{#if sheetCurrent === 'expand'}
			<div
				id="lg-results-body"
				class="flex-1 overflow-y-auto px-4 py-4 sm:px-6"
				style="overscroll-behavior: contain;"
				bind:this={bodyEl}
			>
				<div
					class="grid w-full gap-3"
					style="grid-template-columns: repeat(auto-fit, minmax(min(100%, 28rem), 1fr));"
				>
					{#each ordered as r (r.routerId)}
						<ResultCard result={r} />
					{/each}
				</div>
			</div>
		{/if}
	</aside>
{/if}
