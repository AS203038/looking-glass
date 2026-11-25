<script lang="ts">
	import type { Pb } from '$lib/grpc';
	import { formatBgpAge } from '$lib/bgp';
	import { computeVirtualWindow } from '$lib/virtual';
	import { pivotQuery } from '$lib/stores/query';

	interface Props {
		paths: Pb.BGPPaths;
		nextPageToken?: string;
		loadingMore?: boolean;
		onLoadMore?: () => void;
	}
	let { paths, nextPageToken = '', loadingMore = false, onLoadMore }: Props = $props();

	type SortKey = 'prefix' | 'med' | 'localPref' | 'ageSeconds';
	let searchQuery = $state('');
	let sortKey = $state<SortKey | null>(null);
	let sortAsc = $state(true);

	function toggleSort(key: SortKey) {
		if (sortKey === key) {
			sortAsc = !sortAsc;
		} else {
			sortKey = key;
			sortAsc = true;
		}
	}

	const rawRows = $derived(paths.paths ?? []);
	const hasOrigin = $derived(rawRows.some((p) => !!p.origin));
	const hasAge = $derived(rawRows.some((p) => p.ageSeconds > 0n));

	const filteredRows = $derived.by(() => {
		const q = searchQuery.trim().toLowerCase();
		if (!q) return rawRows;
		return rawRows.filter((p) => {
			if (p.prefix?.toLowerCase().includes(q)) return true;
			if (p.nexthop?.toLowerCase().includes(q)) return true;
			if (p.peerIp?.toLowerCase().includes(q)) return true;
			if (p.peerAsn && String(p.peerAsn).includes(q)) return true;
			if ((p.asPath ?? []).some((asn) => String(asn).includes(q))) return true;
			if ((p.communities ?? []).some((c) => c.toLowerCase().includes(q))) return true;
			if ((p.largeCommunities ?? []).some((c) => c.toLowerCase().includes(q))) return true;
			return false;
		});
	});

	const rows = $derived.by(() => {
		if (!sortKey) return filteredRows;
		const key = sortKey;
		const dir = sortAsc ? 1 : -1;
		return [...filteredRows].sort((a, b) => {
			switch (key) {
				case 'prefix':
					return a.prefix.localeCompare(b.prefix) * dir;
				case 'med':
					return (a.med - b.med) * dir;
				case 'localPref':
					return (a.localPref - b.localPref) * dir;
				case 'ageSeconds':
					return (Number(a.ageSeconds) - Number(b.ageSeconds)) * dir;
			}
		});
	});

	const total = $derived(rows.length);
	const ROW_PX = 28;
	let viewPx = $state(384);
	let scrollEl: HTMLDivElement | undefined = $state();
	let scrollTop = $state(0);

	function onScroll() {
		if (scrollEl) scrollTop = scrollEl.scrollTop;
	}

	$effect(() => {
		void rows;
		if (scrollEl) {
			scrollEl.scrollTop = 0;
		}
		scrollTop = 0;
	});

	const win = $derived(computeVirtualWindow(total, scrollTop, viewPx, { rowPx: ROW_PX }));
	const visible = $derived(win.virtualized ? rows.slice(win.startIdx, win.endIdx) : rows);

	const colSpan = $derived(7 + (hasOrigin ? 1 : 0) + (hasAge ? 1 : 0));

	function shortOrigin(origin: string): string {
		switch (origin.toLowerCase()) {
			case 'igp':
				return 'i';
			case 'egp':
				return 'e';
			case 'incomplete':
				return '?';
			default:
				return origin || '—';
		}
	}
</script>

<div
	class="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	<span>
		{#if searchQuery}
			{rows.length} of {rawRows.length} paths
		{:else}
			{rawRows.length} path{rawRows.length === 1 ? '' : 's'}
		{/if}
	</span>
	<input
		type="search"
		placeholder="Filter prefix, ASN, community…"
		aria-label="Filter BGP paths"
		bind:value={searchQuery}
		class="lg-input w-56 !py-1 text-xs font-mono"
	/>
</div>

<div
	bind:this={scrollEl}
	bind:clientHeight={viewPx}
	onscroll={onScroll}
	class="flex-1 overflow-auto"
>
	<table class="w-full font-mono text-xs tabular-nums">
		<thead
			class="sticky top-0 z-10"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="px-3 py-2 font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('prefix')}>
						Prefix{sortKey === 'prefix' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Next-hop</th>
				<th
					class="px-3 py-2 text-right font-normal tracking-wider uppercase"
					title="Multi-Exit Discriminator (MED)"
				>
					<button type="button" class="hover:underline" onclick={() => toggleSort('med')}>
						MED{sortKey === 'med' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase" title="LOCAL_PREF">
					<button type="button" class="hover:underline" onclick={() => toggleSort('localPref')}>
						LocPref{sortKey === 'localPref' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				{#if hasOrigin}
					<th class="px-3 py-2 font-normal tracking-wider uppercase" title="BGP Origin">Org</th>
				{/if}
				<th class="px-3 py-2 font-normal tracking-wider uppercase">AS-path</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Peer</th>
				{#if hasAge}
					<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">
						<button type="button" class="hover:underline" onclick={() => toggleSort('ageSeconds')}>
							Age{sortKey === 'ageSeconds' ? (sortAsc ? ' ↑' : ' ↓') : ''}
						</button>
					</th>
				{/if}
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Communities</th>
			</tr>
		</thead>
		<tbody>
			{#if win.padTopPx > 0}
				<tr aria-hidden="true" style="height: {win.padTopPx}px;"><td colspan={colSpan}></td></tr>
			{/if}
			{#each visible as p, i (win.startIdx + i)}
				<tr
					class="border-t align-top"
					style={win.virtualized
						? `border-color: var(--color-border); height: ${ROW_PX}px;`
						: 'border-color: var(--color-border);'}
				>
					<td class="px-3 py-1 whitespace-nowrap">
						{#if p.best}
							<span class="mr-1" style="color: var(--color-success);" title="best">▸</span>
						{/if}
						<button
							type="button"
							class="text-left hover:underline"
							title={`Pivot: bgp_route ${p.prefix}`}
							onclick={() => pivotQuery({ command: 'bgp_route', parameter: p.prefix })}
						>
							{p.prefix}
						</button>
					</td>
					<td class="px-3 py-1 whitespace-nowrap">
						{#if p.nexthop}
							<button
								type="button"
								class="text-left hover:underline"
								title={`Pivot: ping ${p.nexthop}`}
								onclick={() => pivotQuery({ command: 'ping', parameter: p.nexthop })}
							>
								{p.nexthop}
							</button>
						{:else}
							—
						{/if}
					</td>
					<td
						class="px-3 py-1 text-right whitespace-nowrap tabular-nums"
						style="color: var(--color-fg-muted);"
					>
						{p.med ? p.med : '—'}
					</td>
					<td
						class="px-3 py-1 text-right whitespace-nowrap tabular-nums"
						style="color: var(--color-fg-muted);"
					>
						{p.localPref ? p.localPref : '—'}
					</td>
					{#if hasOrigin}
						<td class="px-3 py-1 whitespace-nowrap" title={p.origin}>
							{shortOrigin(p.origin)}
						</td>
					{/if}
					<td class="px-3 py-1 whitespace-nowrap" style="color: var(--color-fg-muted);">
						{(p.asPath ?? []).join(' ') || '—'}
					</td>
					<td class="px-3 py-1 whitespace-nowrap" style="color: var(--color-fg-muted);">
						{#if p.peerAsn}AS{p.peerAsn}{/if}
						{#if p.peerIp}<span class="ml-1">{p.peerIp}</span>{/if}
						{#if !p.peerAsn && !p.peerIp}—{/if}
					</td>
					{#if hasAge}
						<td
							class="px-3 py-1 text-right whitespace-nowrap"
							style="color: var(--color-fg-muted);"
						>
							{formatBgpAge(p.ageSeconds)}
						</td>
					{/if}
					<td class={win.virtualized ? 'truncate px-3 py-1 whitespace-nowrap' : 'px-3 py-1'}>
						{#if (p.communities ?? []).length === 0 && (p.largeCommunities ?? []).length === 0}
							<span style="color: var(--color-fg-subtle);">—</span>
						{:else if win.virtualized}
							<span
								class="inline-block max-w-full truncate align-bottom"
								title={[...(p.communities ?? []), ...(p.largeCommunities ?? [])].join(' ')}
							>
								{#each p.communities ?? [] as c, ci (ci)}{#if ci > 0}<span> </span>{/if}<button
										type="button"
										class="lg-badge !px-1.5 !py-0 text-[10px] hover:underline"
										title={`Pivot: bgp_community ${c}`}
										onclick={() => pivotQuery({ command: 'bgp_community', parameter: c })}
										>{c}</button
									>{/each}{#each p.largeCommunities ?? [] as c, ci (ci)}{#if ci > 0 || (p.communities ?? []).length > 0}<span
										>
										</span>{/if}<button
										type="button"
										class="lg-badge lg-badge-info !px-1.5 !py-0 text-[10px] hover:underline"
										title={`Pivot: bgp_community ${c}`}
										onclick={() => pivotQuery({ command: 'bgp_community', parameter: c })}
										>{c}</button
									>{/each}
							</span>
						{:else}
							<span class="inline-flex flex-wrap gap-1">
								{#each p.communities ?? [] as c, ci (ci)}
									<button
										type="button"
										class="lg-badge !px-1.5 !py-0 text-[10px] hover:underline"
										title={`Pivot: bgp_community ${c}`}
										onclick={() => pivotQuery({ command: 'bgp_community', parameter: c })}
										>{c}</button
									>
								{/each}
								{#each p.largeCommunities ?? [] as c, ci (ci)}
									<button
										type="button"
										class="lg-badge lg-badge-info !px-1.5 !py-0 text-[10px] hover:underline"
										title={`Pivot: bgp_community ${c}`}
										onclick={() => pivotQuery({ command: 'bgp_community', parameter: c })}
										>{c}</button
									>
								{/each}
							</span>
						{/if}
					</td>
				</tr>
			{/each}
			{#if win.padBotPx > 0}
				<tr aria-hidden="true" style="height: {win.padBotPx}px;"><td colspan={colSpan}></td></tr>
			{/if}
		</tbody>
	</table>
</div>
<div
	class="flex items-center justify-between gap-2 border-t px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	<span>
		{rows.length} path{rows.length === 1 ? '' : 's'}
		{#if win.virtualized}
			<span class="ml-2" style="color: var(--color-fg-subtle);"
				>· virtualized — scroll to load more rows</span
			>
		{/if}
	</span>
	{#if nextPageToken && onLoadMore}
		<button
			type="button"
			class="lg-btn lg-btn-outline h-7 !px-2.5 text-xs"
			disabled={loadingMore}
			onclick={onLoadMore}
		>
			{loadingMore ? 'Loading…' : 'Load more routes'}
		</button>
	{/if}
</div>
