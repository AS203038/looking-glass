<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		paths: Pb.BGPPaths;
	}
	let { paths }: Props = $props();

	const rows = $derived(paths.paths ?? []);
	const total = $derived(rows.length);

	const VIRTUAL_THRESHOLD = 200;
	const virtualized = $derived(total > VIRTUAL_THRESHOLD);

	const ROW_PX = 28;
	const VIEW_PX = 384;
	const OVERSCAN = 6;

	let scrollEl: HTMLDivElement | undefined = $state();
	let scrollTop = $state(0);

	function onScroll() {
		if (scrollEl) scrollTop = scrollEl.scrollTop;
	}

	$effect(() => {
		void rows;
		if (scrollEl) {
			scrollEl.scrollTop = 0;
			scrollTop = 0;
		}
	});

	const startIdx = $derived(
		virtualized ? Math.max(0, Math.floor(scrollTop / ROW_PX) - OVERSCAN) : 0
	);
	const visibleCount = $derived(Math.ceil(VIEW_PX / ROW_PX) + 2 * OVERSCAN);
	const endIdx = $derived(virtualized ? Math.min(total, startIdx + visibleCount) : total);
	const visible = $derived(virtualized ? rows.slice(startIdx, endIdx) : rows);
	const padTopPx = $derived(virtualized ? startIdx * ROW_PX : 0);
	const padBotPx = $derived(virtualized ? Math.max(0, (total - endIdx) * ROW_PX) : 0);
</script>

<div
	bind:this={scrollEl}
	onscroll={onScroll}
	class="overflow-auto"
	style="max-height: {VIEW_PX}px;"
>
	<table class="w-full font-mono text-xs">
		<thead
			class="sticky top-0 z-10"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Prefix</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Next-hop</th>
				<th
					class="px-3 py-2 text-right font-normal tracking-wider uppercase"
					title="Multi-Exit Discriminator (MED)"
				>
					MED
				</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase" title="LOCAL_PREF">
					LocPref
				</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">AS-path</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Peer</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Communities</th>
			</tr>
		</thead>
		<tbody>
			{#if padTopPx > 0}
				<tr aria-hidden="true" style="height: {padTopPx}px;"><td colspan="7"></td></tr>
			{/if}
			{#each visible as p, i (startIdx + i)}
				<tr
					class="border-t align-top"
					style={virtualized
						? `border-color: var(--color-border); height: ${ROW_PX}px;`
						: 'border-color: var(--color-border);'}
				>
					<td class="px-3 py-1 whitespace-nowrap">
						{#if p.best}
							<span class="mr-1" style="color: var(--color-success);" title="best">▸</span>
						{/if}
						{p.prefix}
					</td>
					<td class="px-3 py-1 whitespace-nowrap">{p.nexthop || '—'}</td>
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
					<td class="px-3 py-1 whitespace-nowrap" style="color: var(--color-fg-muted);">
						{(p.asPath ?? []).join(' ') || '—'}
					</td>
					<td class="px-3 py-1 whitespace-nowrap" style="color: var(--color-fg-muted);">
						{#if p.peerAsn}AS{p.peerAsn}{/if}
						{#if p.peerIp}<span class="ml-1">{p.peerIp}</span>{/if}
						{#if !p.peerAsn && !p.peerIp}—{/if}
					</td>
					<td class={virtualized ? 'truncate px-3 py-1 whitespace-nowrap' : 'px-3 py-1'}>
						{#if (p.communities ?? []).length === 0 && (p.largeCommunities ?? []).length === 0}
							<span style="color: var(--color-fg-subtle);">—</span>
						{:else if virtualized}
							<span
								class="inline-block max-w-full truncate align-bottom"
								title={[...(p.communities ?? []), ...(p.largeCommunities ?? [])].join(' ')}
							>
								{#each p.communities ?? [] as c, ci (c)}{#if ci > 0}<span> </span>{/if}<span
										class="lg-badge !px-1.5 !py-0 text-[10px]"
										title="standard community">{c}</span
									>{/each}{#each p.largeCommunities ?? [] as c, ci (c)}{#if ci > 0 || (p.communities ?? []).length > 0}<span
										>
										</span>{/if}<span
										class="lg-badge lg-badge-info !px-1.5 !py-0 text-[10px]"
										title="large community">{c}</span
									>{/each}
							</span>
						{:else}
							<span class="inline-flex flex-wrap gap-1">
								{#each p.communities ?? [] as c (c)}
									<span class="lg-badge !px-1.5 !py-0 text-[10px]" title="standard community"
										>{c}</span
									>
								{/each}
								{#each p.largeCommunities ?? [] as c (c)}
									<span
										class="lg-badge lg-badge-info !px-1.5 !py-0 text-[10px]"
										title="large community">{c}</span
									>
								{/each}
							</span>
						{/if}
					</td>
				</tr>
			{/each}
			{#if padBotPx > 0}
				<tr aria-hidden="true" style="height: {padBotPx}px;"><td colspan="7"></td></tr>
			{/if}
		</tbody>
	</table>
</div>
<div
	class="border-t px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	{rows.length} path{rows.length === 1 ? '' : 's'}
	{#if virtualized}
		<span class="ml-2" style="color: var(--color-fg-subtle);"
			>· virtualised — scroll to load more rows</span
		>
	{/if}
</div>
