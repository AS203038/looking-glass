<script lang="ts">
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { browser } from '$app/env';
	import {
		routers,
		loadState,
		loadError,
		selectedIds,
		selectedRouters,
		toggleSelection,
		selectMany,
		deselectMany,
		clearSelection,
		loadRouters,
		retryLoadRouters
	} from '$lib/stores/routers';
	import { command, parameter } from '$lib/stores/query';
	import { parseUrlState, syncUrlState } from '$lib/urlSync';
	import { transitionDuration } from '$lib/motion';
	import type { Pb } from '$lib/grpc';
	import Loader from './Loader.svelte';
	import { now, relativeTime, absoluteTime, isoTime } from '$lib/time';
	import Search from '@lucide/svelte/icons/search';
	import MapPin from '@lucide/svelte/icons/map-pin';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import Check from '@lucide/svelte/icons/check';
	import CircleX from '@lucide/svelte/icons/circle-x';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import RouterIcon from '@lucide/svelte/icons/router';
	import CheckCheck from '@lucide/svelte/icons/check-check';
	import EraserIcon from '@lucide/svelte/icons/eraser';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';

	let query = $state('');
	let hideUnhealthy = $state(false);
	let bmpOnly = $state(false);
	let collapsedLocations: Set<string> = $state(new Set());
	let activeLocation = $state<string | null>(null);
	let urlHydrated = $state(false);

	onMount(() => {
		void loadRouters();
	});

	const hasAnyBmp = $derived($routers.some((r) => r.bmpActive));
	const healthyRouters = $derived($routers.filter((r) => r.health?.healthy === true));
	const allHealthySelected = $derived(
		healthyRouters.length > 0 && healthyRouters.every((r) => $selectedIds.has(r.id))
	);

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return $routers.filter((r: Pb.Router) => {
			if (hideUnhealthy && r.health?.healthy !== true) return false;
			if (bmpOnly && !r.bmpActive) return false;
			if (!q) return true;
			return r.name.toLowerCase().includes(q) || (r.location ?? '').toLowerCase().includes(q);
		});
	});

	const grouped = $derived.by(() => {
		const groups = new Map<string, Pb.Router[]>();
		for (const r of filtered) {
			const loc = r.location || 'Other';
			if (!groups.has(loc)) groups.set(loc, []);
			groups.get(loc)!.push(r);
		}
		return Array.from(groups.entries());
	});

	const totalLocationCount = $derived.by(() => {
		const seen = new Set<string>();
		for (const r of $routers) seen.add(r.location || 'Other');
		return seen.size;
	});
	const showQuickNav = $derived(totalLocationCount >= 8);

	$effect(() => {
		if (!browser || urlHydrated || $loadState !== 'ready') return;
		urlHydrated = true;
		const initial = parseUrlState(window.location.search);
		if (initial.routers.length > 0) {
			const wanted = new Set(initial.routers.map((s) => s.toLowerCase()));
			const matched = $routers
				.filter(
					(r) =>
						r.health?.healthy === true &&
						(wanted.has(r.name.toLowerCase()) || wanted.has(r.id.toString()))
				)
				.map((r) => r.id);
			if (matched.length > 0) {
				selectMany(matched);
			}
		}
		if (initial.cmd) {
			command.set(initial.cmd);
		}
		if (initial.param) {
			parameter.set(initial.param);
		}
	});

	$effect(() => {
		if (!browser || !urlHydrated) return;
		syncUrlState({
			routerNames: $selectedRouters.map((r) => r.name),
			cmd: $command,
			param: $parameter
		});
	});

	function toggleGroup(loc: string) {
		const next = new Set(collapsedLocations);
		if (next.has(loc)) next.delete(loc);
		else next.add(loc);
		collapsedLocations = next;
	}

	function isExpanded(loc: string): boolean {
		if (grouped.length === 1) return true;
		return !collapsedLocations.has(loc);
	}

	function selectAllHealthy() {
		selectMany(healthyRouters.map((r) => r.id));
	}

	function selectAllInGroup(group: Pb.Router[]) {
		selectMany(group.filter((r) => r.health?.healthy === true).map((r) => r.id));
	}

	function clearGroup(group: Pb.Router[]) {
		deselectMany(group.map((r) => r.id));
	}

	function groupSelectionState(group: Pb.Router[]): 'none' | 'some' | 'all' {
		const healthy = group.filter((r) => r.health?.healthy === true);
		if (healthy.length === 0) return 'none';
		const selectedInGroup = healthy.filter((r) => $selectedIds.has(r.id)).length;
		if (selectedInGroup === 0) return 'none';
		if (selectedInGroup === healthy.length) return 'all';
		return 'some';
	}

	function locationAnchorId(loc: string): string {
		return `lg-loc-${loc.replace(/[^a-zA-Z0-9_-]+/g, '_')}`;
	}

	function scrollToLocation(loc: string) {
		const el = document.getElementById(locationAnchorId(loc));
		if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	let observer: IntersectionObserver | null = null;
	$effect(() => {
		if (typeof window === 'undefined' || !showQuickNav) {
			observer?.disconnect();
			observer = null;
			return;
		}
		const currentGrouped = grouped;
		observer?.disconnect();
		observer = new IntersectionObserver(
			(entries) => {
				const visible = entries
					.filter((e) => e.isIntersecting)
					.sort((a, b) => b.intersectionRatio - a.intersectionRatio);
				if (visible.length > 0) {
					const id = visible[0].target.id;
					const match = currentGrouped.find(([loc]) => locationAnchorId(loc) === id);
					if (match) activeLocation = match[0];
				}
			},
			{
				rootMargin: '-72px 0px -60% 0px',
				threshold: [0, 0.25, 0.5]
			}
		);
		const rafId = requestAnimationFrame(() => {
			for (const [loc] of currentGrouped) {
				const el = document.getElementById(locationAnchorId(loc));
				if (el) observer?.observe(el);
			}
		});
		return () => {
			cancelAnimationFrame(rafId);
			observer?.disconnect();
		};
	});

	/** Health check Date from the protobuf timestamp, or null when missing. */
	function healthCheckDate(r: Pb.Router): Date | null {
		const ts = r.health?.timestamp;
		if (!ts) return null;
		return new Date(Number(ts.seconds) * 1000);
	}
</script>

<section aria-labelledby="router-picker-title" class="flex w-full flex-col gap-3">
	<header class="flex flex-wrap items-center gap-2">
		<h2 id="router-picker-title" class="text-sm font-semibold tracking-wide uppercase">Routers</h2>
		{#if $loadState === 'ready'}
			<span class="lg-badge">{$routers.length} total</span>
			{#if $selectedIds.size > 0}
				<span class="lg-badge lg-badge-info">{$selectedIds.size} selected</span>
			{/if}
			<div class="ml-auto flex flex-wrap items-center gap-1.5">
				<button
					type="button"
					class="lg-btn h-7 !px-2 text-xs {hideUnhealthy ? 'lg-btn-outline' : 'lg-btn-ghost'}"
					aria-pressed={hideUnhealthy}
					onclick={() => (hideUnhealthy = !hideUnhealthy)}
				>
					Hide unreachable
				</button>
				{#if hasAnyBmp}
					<button
						type="button"
						class="lg-btn h-7 !px-2 text-xs {bmpOnly ? 'lg-btn-outline' : 'lg-btn-ghost'}"
						aria-pressed={bmpOnly}
						onclick={() => (bmpOnly = !bmpOnly)}
					>
						BMP only
					</button>
				{/if}
				{#if healthyRouters.length > 0 && !allHealthySelected}
					<button
						type="button"
						class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
						onclick={selectAllHealthy}
						title="Select all healthy routers"
					>
						<CheckCheck size={12} />
						Select all healthy
					</button>
				{/if}
				{#if $selectedIds.size > 0}
					<button
						type="button"
						class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
						onclick={clearSelection}
						title="Deselect every router"
					>
						Clear all
					</button>
				{/if}
			</div>
		{/if}
	</header>

	{#if $loadState === 'loading' && $routers.length === 0}
		<div class="lg-card flex justify-center p-8" in:fade={{ duration: transitionDuration(150) }}>
			<Loader label="loading routers" />
		</div>
	{:else if $loadState === 'error'}
		<div class="lg-card flex flex-wrap items-center gap-3 p-4" style="color: var(--color-danger);">
			<CircleX size={18} class="shrink-0" />
			<div class="min-w-0 flex-1">
				<p class="text-sm font-medium">Failed to load routers</p>
				{#if $loadError}
					<p class="font-mono text-xs break-words opacity-80">{$loadError}</p>
				{/if}
			</div>
			<button type="button" class="lg-btn lg-btn-outline h-8 text-xs" onclick={retryLoadRouters}>
				<RotateCcw size={13} />
				Retry
			</button>
		</div>
	{:else if $loadState === 'ready' && $routers.length === 0}
		<div class="lg-card p-6 text-center text-sm" style="color: var(--color-fg-muted);">
			No routers configured on this Looking Glass instance.
		</div>
	{:else}
		<div
			class="sticky z-10 flex items-center backdrop-blur-md"
			style="top: var(--lg-header-h);
			       height: var(--lg-picker-search-h);
			       background-color: color-mix(in oklab, var(--color-bg) 85%, transparent);"
		>
			<div class="relative w-full">
				<Search
					size={16}
					class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 opacity-60"
				/>
				<input
					type="search"
					class="lg-input pl-9"
					placeholder="Filter by name or location…"
					bind:value={query}
					aria-label="Filter routers"
				/>
			</div>
		</div>

		{#if grouped.length === 0}
			<p class="px-1 py-6 text-center text-sm" style="color: var(--color-fg-muted);">
				{#if query.trim()}
					No routers match <span class="font-mono">"{query}"</span>.
				{:else}
					No routers match the active filters.
				{/if}
			</p>
		{:else}
			<div class="flex gap-3">
				<div class="flex min-w-0 flex-1 flex-col gap-3">
					{#each grouped as [location, routerGroup] (location)}
						{@const expanded = isExpanded(location)}
						{@const selState = groupSelectionState(routerGroup)}
						{@const anchorId = locationAnchorId(location)}
						<div class="lg-card scroll-mt-[120px]" id={anchorId}>
							<div
								class="sticky z-[5] flex items-center gap-2 rounded-t-xl border-b px-4 py-2.5 backdrop-blur-md"
								style="top: calc(var(--lg-header-h) + var(--lg-picker-search-h));
								       background-color: color-mix(in oklab, var(--color-bg-elevated) 95%, transparent);
								       border-color: var(--color-border);"
							>
								<button
									type="button"
									class="flex flex-1 items-center gap-2 text-left text-sm font-medium hover:opacity-80"
									onclick={() => toggleGroup(location)}
									aria-expanded={expanded}
									aria-controls={`${anchorId}-content`}
								>
									<MapPin size={14} class="opacity-60" />
									<span class="capitalize">{location}</span>
									<span class="text-xs" style="color: var(--color-fg-subtle);">
										{routerGroup.length}
									</span>
									<ChevronDown
										size={16}
										class="opacity-60 transition-transform {expanded ? 'rotate-0' : '-rotate-90'}"
									/>
								</button>

								{#if expanded && routerGroup.some((r) => r.health?.healthy === true)}
									{#if selState !== 'all'}
										<button
											type="button"
											class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
											onclick={() => selectAllInGroup(routerGroup)}
											title="Select all healthy routers in {location}"
										>
											<CheckCheck size={12} />
											<span class="hidden sm:inline">Select all</span>
										</button>
									{/if}
									{#if selState !== 'none'}
										<button
											type="button"
											class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
											onclick={() => clearGroup(routerGroup)}
											title="Deselect all routers in {location}"
										>
											<EraserIcon size={12} />
											<span class="hidden sm:inline">Clear</span>
										</button>
									{/if}
								{/if}
							</div>

							{#if expanded}
								<div
									id={`${anchorId}-content`}
									class="grid gap-2 p-3"
									style="grid-template-columns: repeat(auto-fill, minmax(min(100%, 14rem), 1fr));"
								>
									{#each routerGroup as rt (rt.id)}
										{@const selected = $selectedIds.has(rt.id)}
										{@const healthy = rt.health?.healthy === true}
										{@const lastCheck = healthCheckDate(rt)}
										<button
											type="button"
											data-router-id={rt.id.toString()}
											class="group relative flex scroll-mt-[140px] flex-col items-start gap-1 rounded-lg border px-3 py-2.5 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50"
											style="border-color: {selected
												? 'var(--color-accent)'
												: 'var(--color-border)'};
											       background-color: {selected
												? 'color-mix(in oklab, var(--color-accent) 12%, transparent)'
												: 'transparent'};"
											onclick={() => healthy && toggleSelection(rt.id)}
											disabled={!healthy}
											aria-pressed={selected}
											aria-label={`${rt.name} — ${healthy ? 'healthy' : 'unhealthy'}`}
										>
											<span class="flex w-full items-center gap-2">
												<RouterIcon size={14} class="shrink-0 opacity-60" />
												<span class="truncate text-sm font-medium">{rt.name}</span>
												{#if rt.bmpActive}
													<span
														class="lg-badge lg-badge-info !px-1.5 !py-0 text-[10px]"
														title="BMP real-time telemetry active"
													>
														BMP
													</span>
												{/if}
												{#if selected}
													<span
														class="ml-auto inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full"
														style="background-color: var(--color-accent);
														       color: var(--color-on-accent);"
														aria-hidden="true"
													>
														<Check size={12} />
													</span>
												{/if}
											</span>

											<span
												class="flex w-full items-center gap-1.5 text-xs"
												style="color: var(--color-fg-muted);"
											>
												{#if healthy}
													<CircleCheck
														size={12}
														style="color: var(--color-success);"
														aria-hidden="true"
													/>
													<span>healthy</span>
												{:else}
													<CircleX
														size={12}
														style="color: var(--color-danger);"
														aria-hidden="true"
													/>
													<span>unreachable</span>
												{/if}
												{#if lastCheck}
													<span aria-hidden="true">·</span>
													<time
														class="font-mono"
														datetime={isoTime(lastCheck)}
														title={`Last health check: ${absoluteTime(lastCheck)}`}
													>
														{relativeTime(lastCheck, $now)}
													</time>
												{/if}
											</span>
										</button>
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</div>

				{#if showQuickNav}
					<nav
						class="sticky hidden h-max max-h-[calc(100vh-14rem)] w-32 shrink-0 flex-col gap-0.5 overflow-y-auto rounded-lg border p-2 lg:flex"
						style="top: calc(var(--lg-header-h) + var(--lg-picker-search-h) + 8px);
						       background-color: var(--color-bg-elevated);
						       border-color: var(--color-border);"
						aria-label="Jump to location"
					>
						<p
							class="px-1 pt-1 pb-0.5 text-[0.65rem] font-medium tracking-wider uppercase"
							style="color: var(--color-fg-subtle);"
						>
							Jump to
						</p>
						{#each grouped as [loc] (loc)}
							{@const active = activeLocation === loc}
							<button
								type="button"
								class="truncate rounded px-2 py-1 text-left text-xs capitalize transition-colors"
								style="background-color: {active
									? 'color-mix(in oklab, var(--color-accent) 16%, transparent)'
									: 'transparent'};
								       color: {active ? 'var(--color-accent)' : 'var(--color-fg-muted)'};"
								onclick={() => scrollToLocation(loc)}
								aria-current={active ? 'location' : undefined}
								title={loc}
							>
								{loc}
							</button>
						{/each}
					</nav>
				{/if}
			</div>
		{/if}
	{/if}
</section>
