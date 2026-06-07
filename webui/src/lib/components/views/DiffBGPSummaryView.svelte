<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		oldSummary: Pb.BGPSummaryParsed;
		newSummary: Pb.BGPSummaryParsed;
	}
	let { oldSummary, newSummary }: Props = $props();

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

	interface DiffPeer {
		ip: string;
		old?: Pb.BGPPeer;
		new?: Pb.BGPPeer;
		status: 'added' | 'removed' | 'changed' | 'unchanged';
	}

	const rows = $derived.by(() => {
		const map = new Map<string, DiffPeer>();
		for (const p of oldSummary.peers || []) {
			map.set(p.peerIp, { ip: p.peerIp, old: p, status: 'removed' });
		}
		for (const p of newSummary.peers || []) {
			if (map.has(p.peerIp)) {
				const existing = map.get(p.peerIp)!;
				existing.new = p;
				const changed =
					existing.old!.state !== p.state || existing.old!.prefixesReceived !== p.prefixesReceived;
				existing.status = changed ? 'changed' : 'unchanged';
			} else {
				map.set(p.peerIp, { ip: p.peerIp, new: p, status: 'added' });
			}
		}
		return Array.from(map.values()).sort((a, b) => a.ip.localeCompare(b.ip));
	});
</script>

<div class="flex-1 overflow-auto bg-(--color-bg-base)">
	<table class="w-full font-mono text-xs">
		<thead
			class="sticky top-0"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="px-3 py-2 font-normal">Diff</th>
				<th class="px-3 py-2 font-normal">Peer</th>
				<th class="px-3 py-2 font-normal">State</th>
				<th class="px-3 py-2 text-right font-normal">Pfx&nbsp;in</th>
			</tr>
		</thead>
		<tbody>
			{#each rows as r (r.ip)}
				<tr
					class="border-t"
					style="border-color: var(--color-border); background-color: {r.status === 'added'
						? 'color-mix(in oklab, var(--color-success) 10%, transparent)'
						: r.status === 'removed'
							? 'color-mix(in oklab, var(--color-danger) 10%, transparent)'
							: r.status === 'changed'
								? 'color-mix(in oklab, var(--color-warning) 10%, transparent)'
								: 'transparent'};"
				>
					<td class="px-3 py-1.5 font-bold whitespace-nowrap">
						{#if r.status === 'added'}<span style="color: var(--color-success)">+</span>
						{:else if r.status === 'removed'}<span style="color: var(--color-danger)">-</span>
						{:else if r.status === 'changed'}<span style="color: var(--color-warning)">~</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 whitespace-nowrap">
						{r.ip}
						<span style="color: var(--color-fg-subtle);">AS{r.new?.peerAsn || r.old?.peerAsn}</span>
					</td>
					<td class="px-3 py-1.5">
						{#if r.status === 'changed' && r.old?.state !== r.new?.state}
							<span class="mr-1 line-through opacity-50">{r.old?.state}</span>
							<span class="lg-badge {stateColor(r.new!.state)} !px-1.5 !py-0 text-[10px]"
								>{r.new!.state}</span
							>
						{:else}
							<span class="lg-badge {stateColor((r.new || r.old)!.state)} !px-1.5 !py-0 text-[10px]"
								>{(r.new || r.old)!.state}</span
							>
						{/if}
					</td>
					<td class="px-3 py-1.5 text-right">
						{#if r.status === 'changed' && r.old?.prefixesReceived !== r.new?.prefixesReceived}
							<span class="mr-1 line-through opacity-50">{Number(r.old?.prefixesReceived)}</span>
							<span class="font-bold text-(--color-success)">{Number(r.new?.prefixesReceived)}</span
							>
						{:else}
							{Number((r.new || r.old)?.prefixesReceived)}
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
