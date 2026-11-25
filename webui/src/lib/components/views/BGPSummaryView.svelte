<script lang="ts">
	import type { Pb } from '$lib/grpc';
	import { bgpStateBadgeClass, formatBgpAge } from '$lib/bgp';
	import { formatPeerRoutesParam, pivotQuery } from '$lib/stores/query';

	interface Props {
		summary: Pb.BGPSummaryParsed;
		routerId?: string;
	}
	let { summary, routerId }: Props = $props();

	type SortKey =
		'peerIp' | 'peerAsn' | 'state' | 'uptimeSeconds' | 'prefixesReceived' | 'prefixesSent';
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

	const filteredRows = $derived(
		(summary.peers ?? []).filter((p) => {
			if (!searchQuery) return true;
			const q = searchQuery.toLowerCase();
			return (
				p.peerIp?.toLowerCase().includes(q) ||
				p.peerAsn?.toString().includes(q) ||
				p.description?.toLowerCase().includes(q) ||
				p.state?.toLowerCase().includes(q)
			);
		})
	);

	const rows = $derived.by(() => {
		if (!sortKey) return filteredRows;
		const key = sortKey;
		const dir = sortAsc ? 1 : -1;
		return [...filteredRows].sort((a, b) => {
			switch (key) {
				case 'peerAsn':
					return (a.peerAsn - b.peerAsn) * dir;
				case 'uptimeSeconds':
					return (Number(a.uptimeSeconds) - Number(b.uptimeSeconds)) * dir;
				case 'prefixesReceived':
					return (Number(a.prefixesReceived) - Number(b.prefixesReceived)) * dir;
				case 'prefixesSent':
					return (Number(a.prefixesSent) - Number(b.prefixesSent)) * dir;
				case 'state':
					return a.state.localeCompare(b.state) * dir;
				case 'peerIp':
					return a.peerIp.localeCompare(b.peerIp) * dir;
			}
		});
	});

	const hasDescriptions = $derived((summary.peers ?? []).some((p) => !!p.description));
	const hasAccepted = $derived((summary.peers ?? []).some((p) => p.prefixesAccepted > 0n));

	function pivotPeer(peerIp: string) {
		if (!peerIp) return;
		pivotQuery({
			command: 'bgp_peer_routes',
			parameter: formatPeerRoutesParam(peerIp, 'received', ''),
			selectRouterId: routerId
		});
	}
</script>

<div
	class="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	<div class="flex items-center gap-3">
		{#if summary.localAsn}
			<span>local AS <span style="color: var(--color-fg);">{summary.localAsn}</span></span>
		{/if}
		{#if summary.routerId}
			<span>router-id <span style="color: var(--color-fg);">{summary.routerId}</span></span>
		{/if}
	</div>
	<div class="ml-auto flex items-center">
		<input
			type="search"
			placeholder="Search ASN, IP, Desc…"
			aria-label="Filter BGP peers"
			bind:value={searchQuery}
			class="lg-input w-52 !py-1 font-mono text-xs"
		/>
	</div>
</div>

<div class="flex-1 overflow-auto">
	<table class="w-full font-mono text-xs tabular-nums">
		<thead
			class="sticky top-0"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				{#if hasDescriptions}
					<th class="px-3 py-2 font-normal tracking-wider uppercase">Description</th>
				{/if}
				<th class="px-3 py-2 font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('peerIp')}>
						Peer{sortKey === 'peerIp' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('peerAsn')}>
						AS{sortKey === 'peerAsn' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">AF</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('state')}>
						State{sortKey === 'state' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('uptimeSeconds')}>
						Uptime{sortKey === 'uptimeSeconds' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">
					<button
						type="button"
						class="hover:underline"
						onclick={() => toggleSort('prefixesReceived')}
					>
						Pfx&nbsp;in{sortKey === 'prefixesReceived' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
				{#if hasAccepted}
					<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">Pfx&nbsp;acc</th>
				{/if}
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">
					<button type="button" class="hover:underline" onclick={() => toggleSort('prefixesSent')}>
						Pfx&nbsp;out{sortKey === 'prefixesSent' ? (sortAsc ? ' ↑' : ' ↓') : ''}
					</button>
				</th>
			</tr>
		</thead>
		<tbody>
			{#each rows as p, i (i)}
				<tr class="border-t" style="border-color: var(--color-border);">
					{#if hasDescriptions}
						<td class="px-3 py-1.5 whitespace-nowrap" style="color: var(--color-fg-subtle);">
							{p.description || '—'}
						</td>
					{/if}
					<td class="px-3 py-1.5 whitespace-nowrap">
						<button
							type="button"
							class="text-left hover:underline"
							title={`Pivot: inspect routes for ${p.peerIp}`}
							onclick={() => pivotPeer(p.peerIp)}
						>
							{p.peerIp}
						</button>
					</td>
					<td class="px-3 py-1.5">{p.peerAsn}</td>
					<td class="px-3 py-1.5" style="color: var(--color-fg-muted);">
						{p.addressFamily?.replace('-unicast', '') ?? '—'}
					</td>
					<td class="px-3 py-1.5">
						<span class="lg-badge {bgpStateBadgeClass(p.state)} !px-1.5 !py-0 text-[10px]">
							{p.state}
						</span>
						{#if p.stateDetail}
							<span class="ml-1" style="color: var(--color-fg-subtle);">{p.stateDetail}</span>
						{/if}
					</td>
					<td class="px-3 py-1.5" style="color: var(--color-fg-muted);">
						{formatBgpAge(p.uptimeSeconds)}
					</td>
					<td class="px-3 py-1.5 text-right">{Number(p.prefixesReceived).toLocaleString()}</td>
					{#if hasAccepted}
						<td class="px-3 py-1.5 text-right">{Number(p.prefixesAccepted).toLocaleString()}</td>
					{/if}
					<td class="px-3 py-1.5 text-right">{Number(p.prefixesSent).toLocaleString()}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
<div
	class="border-t px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	{#if searchQuery}
		Showing {rows.length} of {(summary.peers ?? []).length} peers (filtered)
	{:else}
		{rows.length} peer{rows.length === 1 ? '' : 's'}
	{/if}
</div>
