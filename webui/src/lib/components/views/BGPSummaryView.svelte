<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		summary: Pb.BGPSummaryParsed;
	}
	let { summary }: Props = $props();

	let searchQuery = $state('');

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

	const rows = $derived((summary.peers ?? []).filter((p) => {
		if (!searchQuery) return true;
		const q = searchQuery.toLowerCase();
		return (
			p.peerIp?.toLowerCase().includes(q) ||
			p.peerAsn?.toString().includes(q) ||
			p.description?.toLowerCase().includes(q)
		);
	}));

	const hasDescriptions = $derived((summary.peers ?? []).some((p) => !!p.description));
</script>

{#if summary.localAsn || summary.routerId}
	<div
		class="border-b px-4 py-2 font-mono text-xs flex flex-wrap items-center justify-between gap-2"
		style="border-color: var(--color-border); color: var(--color-fg-muted);"
	>
		<div class="flex items-center gap-3">
			{#if summary.localAsn}<span
					>local AS <span style="color: var(--color-fg);">{summary.localAsn}</span></span
				>{/if}
			{#if summary.routerId}<span
					>router-id <span style="color: var(--color-fg);">{summary.routerId}</span></span
				>{/if}
		</div>
		<div class="flex items-center">
			<input
				type="text"
				placeholder="Search ASN, IP, Desc..."
				bind:value={searchQuery}
				class="px-2 py-0.5 text-xs w-48 bg-transparent border rounded font-mono focus:outline-none"
				style="border-color: var(--color-border); color: var(--color-fg);"
			/>
		</div>
	</div>
{:else}
	<div class="px-4 py-1.5 border-b flex justify-end" style="border-color: var(--color-border);">
		<input
			type="text"
			placeholder="Search ASN, IP, Desc..."
			bind:value={searchQuery}
			class="px-2 py-0.5 text-xs w-48 bg-transparent border rounded font-mono focus:outline-none"
			style="border-color: var(--color-border); color: var(--color-fg);"
		/>
	</div>
{/if}
<div class="flex-1 overflow-auto">
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
	{#if searchQuery}
		Showing {rows.length} of {(summary.peers ?? []).length} peers (filtered)
	{:else}
		{rows.length} peer{rows.length === 1 ? '' : 's'}
	{/if}
</div>
