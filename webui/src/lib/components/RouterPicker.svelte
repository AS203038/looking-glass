<script lang="ts">
	/**
	 * Router selection UI.
	 *
	 * Two scaling problems this design solves:
	 *
	 *   1. Many routers per location — collapsible location sections, one
	 *      render path that scales from 1 to 100s of routers, with inline
	 *      health badges and last-check timestamps for unhealthy routers
	 *      (visible to all pointer types — old version was hover-popup
	 *      only, broken on touch).
	 *
	 *   2. Many locations — three additive affordances:
	 *      a) **Sticky group headers** so the user always sees which
	 *         location they're scrolling through. They glue to the top of
	 *         the viewport (just under the page header) instead of
	 *         disappearing upward with the list.
	 *      b) **Per-group bulk actions** ("Select all" / "Clear") in each
	 *         header — operators who want "all of Frankfurt" can do it in
	 *         one click instead of N.
	 *      c) **Quick-Nav rail** (desktop only, when location count > 8)
	 *         — an iOS-Contacts-style alphabetical jump rail. Each rail
	 *         button scrolls its location into view; current section is
	 *         highlighted via IntersectionObserver.
	 *
	 * Selection chip strip and selection counter live in `<CommandDock>`;
	 * results live in `<ResultsSheet>` anchored above the dock. The picker
	 * stays focused on a single responsibility: choosing routers.
	 *
	 * NB: we deliberately do NOT use `overflow-hidden` on group cards;
	 * any `overflow != visible` ancestor establishes a scroll context and
	 * silently disables `position: sticky` inside it, which is what broke
	 * the sticky group headers in an earlier iteration. To still mask the
	 * header background to the card's rounded corners, the header itself
	 * carries `rounded-t-xl`.
	 */
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import {
		routers,
		loadState,
		selectedIds,
		toggleSelection,
		selectMany,
		clearSelection,
		loadRouters
	} from '$lib/stores/routers';
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

	let allRouters: Pb.Router[] = $state([]);
	let routerLoadState: 'idle' | 'loading' | 'ready' | 'error' = $state('idle');
	let ids: Set<bigint> = $state(new Set());
	let query = $state('');
	let collapsedLocations: Set<string> = $state(new Set());
	let activeLocation = $state<string | null>(null);
	// Subscribe to the ticking `$now` store so every relative-timestamp
	// in the template auto-refreshes every 30 s without per-card setInterval.
	let nowDate = $state(new Date());

	routers.subscribe((r) => (allRouters = r));
	loadState.subscribe((s) => (routerLoadState = s));
	selectedIds.subscribe((s) => (ids = s));
	now.subscribe((d) => (nowDate = d));

	onMount(loadRouters);

	// Group routers by location, applying the search filter once at the top.
	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return allRouters;
		return allRouters.filter(
			(r: Pb.Router) =>
				r.name.toLowerCase().includes(q) || (r.location ?? '').toLowerCase().includes(q)
		);
	});

	const grouped = $derived.by(() => {
		const groups = new Map<string, Pb.Router[]>();
		for (const r of filtered) {
			const loc = r.location || 'Other';
			if (!groups.has(loc)) groups.set(loc, []);
			groups.get(loc)!.push(r);
		}
		// Preserve insertion order; locations appear in the order first seen.
		return Array.from(groups.entries());
	});

	// Quick-Nav rail only earns its keep on desktop with many locations.
	// We expose it from `>= 8` distinct (unfiltered) locations.
	const totalLocationCount = $derived.by(() => {
		const seen = new Set<string>();
		for (const r of allRouters) seen.add(r.location || 'Other');
		return seen.size;
	});
	const showQuickNav = $derived(totalLocationCount >= 8);

	function toggleGroup(loc: string) {
		const next = new Set(collapsedLocations);
		if (next.has(loc)) next.delete(loc);
		else next.add(loc);
		collapsedLocations = next;
	}

	function isExpanded(loc: string): boolean {
		// Auto-expand if there's only one location, or it's been explicitly toggled open.
		if (grouped.length === 1) return true;
		return !collapsedLocations.has(loc);
	}

	function selectAllInGroup(group: Pb.Router[]) {
		// Only healthy routers can run queries, so don't lie to the user by
		// including unhealthy ones in the "Select all" expectation.
		selectMany(group.filter((r) => r.health?.healthy === true).map((r) => r.id));
	}

	function clearGroup(group: Pb.Router[]) {
		// Inverse of selectAllInGroup — remove only this group's routers from
		// the selection, leaving other locations' selections intact.
		const groupIds = new Set(group.map((r) => r.id));
		for (const id of groupIds) {
			if (ids.has(id)) toggleSelection(id);
		}
	}

	function groupSelectionState(group: Pb.Router[]): 'none' | 'some' | 'all' {
		const healthy = group.filter((r) => r.health?.healthy === true);
		if (healthy.length === 0) return 'none';
		const selectedInGroup = healthy.filter((r) => ids.has(r.id)).length;
		if (selectedInGroup === 0) return 'none';
		if (selectedInGroup === healthy.length) return 'all';
		return 'some';
	}

	function locationAnchorId(loc: string): string {
		// Stable slug for IntersectionObserver targets and Quick-Nav links.
		return `lg-loc-${loc.replace(/[^a-zA-Z0-9_-]+/g, '_')}`;
	}

	function scrollToLocation(loc: string) {
		const el = document.getElementById(locationAnchorId(loc));
		if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	// Track which location is currently visible to highlight the Quick-Nav.
	let observer: IntersectionObserver | null = null;
	$effect(() => {
		if (typeof window === 'undefined') return;
		observer?.disconnect();
		observer = new IntersectionObserver(
			(entries) => {
				const visible = entries
					.filter((e) => e.isIntersecting)
					.sort((a, b) => b.intersectionRatio - a.intersectionRatio);
				if (visible.length > 0) {
					const id = visible[0].target.id;
					const match = grouped.find(([loc]) => locationAnchorId(loc) === id);
					if (match) activeLocation = match[0];
				}
			},
			{
				rootMargin: '-72px 0px -60% 0px',
				threshold: [0, 0.25, 0.5]
			}
		);
		requestAnimationFrame(() => {
			for (const [loc] of grouped) {
				const el = document.getElementById(locationAnchorId(loc));
				if (el) observer!.observe(el);
			}
		});
		return () => observer?.disconnect();
	});

	/** Health check Date from the protobuf timestamp, or null when missing. */
	function healthCheckDate(r: Pb.Router): Date | null {
		const ts = r.health?.timestamp;
		if (!ts) return null;
		return new Date(Number(ts.seconds) * 1000);
	}
</script>

<section aria-labelledby="router-picker-title" class="flex w-full flex-col gap-3">
	<header class="flex flex-wrap items-baseline gap-3" id="router-picker-title">
		<h2 class="text-sm font-semibold tracking-wide uppercase">Routers</h2>
		{#if routerLoadState === 'ready'}
			<span class="lg-badge">{allRouters.length} total</span>
			{#if ids.size > 0}
				<span class="lg-badge lg-badge-info">{ids.size} selected</span>
				<button
					type="button"
					class="lg-btn lg-btn-ghost ml-auto h-7 !px-2 text-xs"
					onclick={clearSelection}
					title="Deselect every router"
				>
					Clear all
				</button>
			{/if}
		{/if}
	</header>

	{#if routerLoadState === 'loading' && allRouters.length === 0}
		<div class="lg-card flex justify-center p-8" in:fade>
			<Loader label="loading routers" />
		</div>
	{:else if routerLoadState === 'error'}
		<div class="lg-card flex items-center gap-3 p-4" style="color: var(--color-danger);">
			<CircleX size={18} />
			<span class="text-sm">Failed to load routers — see the toast for details.</span>
		</div>
	{:else}
		<!-- Sticky search bar — pinned just under the page header. -->
		<div
			class="sticky top-14 z-10 flex h-14 items-center backdrop-blur-md"
			style="background-color: color-mix(in oklab, var(--color-bg) 85%, transparent);"
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
				No routers match <span class="font-mono">"{query}"</span>.
			</p>
		{:else}
			<div class="flex gap-3">
				<!-- Main scrollable column.
					 NOTE: NO `overflow-hidden` on the group cards — any
					 overflow != visible silently disables `position: sticky`
					 on descendants. -->
				<div class="flex min-w-0 flex-1 flex-col gap-3">
					{#each grouped as [location, routerGroup] (location)}
						{@const expanded = isExpanded(location)}
						{@const selState = groupSelectionState(routerGroup)}
						{@const anchorId = locationAnchorId(location)}
						<div class="lg-card" id={anchorId}>
							<!-- Sticky location header — lands just under the sticky
								 search row above (56 + 56 = 112 px). The header carries
								 its own `rounded-t-xl` so it visually clips to the
								 card's rounded top corners without needing
								 `overflow-hidden` on the parent (which would break
								 sticky). -->
							<div
								class="sticky top-[112px] z-[5] flex items-center gap-2 rounded-t-xl border-b px-4 py-2.5 backdrop-blur-md"
								style="background-color: color-mix(in oklab, var(--color-bg-elevated) 95%, transparent);
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
										{@const selected = ids.has(rt.id)}
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
														{relativeTime(lastCheck, nowDate)}
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

				<!-- Quick-Nav rail (desktop only, ≥ 8 locations). -->
				{#if showQuickNav}
					<nav
						class="sticky top-[120px] hidden h-max max-h-[calc(100vh-14rem)] w-32 shrink-0 flex-col gap-0.5 overflow-y-auto rounded-lg border p-2 lg:flex"
						style="background-color: var(--color-bg-elevated);
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
								aria-current={active ? 'true' : 'false'}
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
