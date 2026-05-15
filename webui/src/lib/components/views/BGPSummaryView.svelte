<script lang="ts">
	/**
	 * Structured BGP-summary view.
	 *
	 * Renders the parsed [Pb.BGPSummaryParsed] as a per-peer table
	 * with state-coloured badges. Header strip carries the local
	 * router-id / ASN when reported by the parser.
	 */
	import type { Pb } from '$lib/grpc';

	interface Props {
		summary: Pb.BGPSummaryParsed;
	}
	let { summary }: Props = $props();

	function stateColor(state: string): string {
		switch (state) {
			case 'established':
				return 'lg-badge-success';
			case 'idle':
			case 'connect':
				return 'lg-badge-warning';
			case 'active':
			case 'opensent':
			case 'openconfirm':
				return 'lg-badge-info';
			default:
				return 'lg-badge-danger';
		}
	}

	function fmtAge(seconds: bigint): string {
		const s = Number(seconds);
		if (!s) return '—';
		const d = Math.floor(s / 86400);
		const h = Math.floor((s % 86400) / 3600);
		const m = Math.floor((s % 3600) / 60);
		if (d > 0) return `${d}d${h}h`;
		if (h > 0) return `${h}h${m}m`;
		return `${m}m`;
	}

	const rows = $derived(summary.peers ?? []);

	// Conditional `Description` column: shown only when at least
	// one peer in the current dataset carries a non-empty
	// `description` (per-vendor templates may or may not expose
	// the operator-configured neighbour description). When no row
	// populates it, the column disappears entirely — keeps the
	// table compact for vendors that don't surface descriptions
	// (Cisco/Juniper/Nokia/FRR), while making them first-class
	// when present (current Arista EOS, MikroTik RouterOS).
	const hasDescriptions = $derived(rows.some((p) => !!p.description));
</script>

{#if summary.localAsn || summary.routerId}
	<div
		class="border-b px-4 py-2 font-mono text-xs"
		style="border-color: var(--color-border); color: var(--color-fg-muted);"
	>
		{#if summary.localAsn}<span
				>local AS <span style="color: var(--color-fg);">{summary.localAsn}</span></span
			>{/if}
		{#if summary.routerId}<span class="ml-3"
				>router-id <span style="color: var(--color-fg);">{summary.routerId}</span></span
			>{/if}
	</div>
{/if}
<div class="max-h-96 overflow-auto">
	<table class="w-full font-mono text-xs">
		<thead
			class="sticky top-0"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				{#if hasDescriptions}
					<th class="px-3 py-2 font-normal tracking-wider uppercase">Description</th>
				{/if}
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Peer</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">AS</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">AF</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">State</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Uptime</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">Pfx&nbsp;in</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">Pfx&nbsp;out</th>
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
					<td class="px-3 py-1.5 whitespace-nowrap">{p.peerIp}</td>
					<td class="px-3 py-1.5">{p.peerAsn}</td>

					<td class="px-3 py-1.5" style="color: var(--color-fg-muted);">
						{p.addressFamily?.replace('-unicast', '') ?? '—'}
					</td>
					<td class="px-3 py-1.5">
						<span class="lg-badge {stateColor(p.state)} !px-1.5 !py-0 text-[10px]">{p.state}</span>
						{#if p.stateDetail}
							<span class="ml-1" style="color: var(--color-fg-subtle);">{p.stateDetail}</span>
						{/if}
					</td>
					<td class="px-3 py-1.5" style="color: var(--color-fg-muted);"
						>{fmtAge(p.uptimeSeconds)}</td
					>
					<td class="px-3 py-1.5 text-right">{Number(p.prefixesReceived)}</td>
					<td class="px-3 py-1.5 text-right">{Number(p.prefixesSent)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
<div
	class="border-t px-4 py-2 font-mono text-xs"
	style="border-color: var(--color-border); color: var(--color-fg-muted);"
>
	{rows.length} peer{rows.length === 1 ? '' : 's'}
</div>
