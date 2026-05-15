<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		stats: Pb.PingStats;
	}
	let { stats }: Props = $props();

	const lossClass = $derived(
		stats.lossPct >= 50
			? 'text-(--color-danger)'
			: stats.lossPct > 0
				? 'text-(--color-warning)'
				: 'text-(--color-success)'
	);
</script>

<div class="grid grid-cols-2 gap-3 px-4 py-3 sm:grid-cols-4">
	<div>
		<div class="text-[10px] tracking-wider uppercase" style="color: var(--color-fg-subtle);">
			Loss
		</div>
		<div class="font-mono text-lg font-semibold {lossClass}">
			{stats.lossPct.toFixed(0)}%
		</div>
		<div class="text-xs" style="color: var(--color-fg-muted);">
			{stats.packetsReceived} / {stats.packetsSent} pkts
		</div>
	</div>
	<div>
		<div class="text-[10px] tracking-wider uppercase" style="color: var(--color-fg-subtle);">
			RTT min
		</div>
		<div class="font-mono text-lg font-semibold">
			{stats.rttMinMs.toFixed(2)}
			<span class="text-xs font-normal" style="color: var(--color-fg-muted);">ms</span>
		</div>
	</div>
	<div>
		<div class="text-[10px] tracking-wider uppercase" style="color: var(--color-fg-subtle);">
			RTT avg
		</div>
		<div class="font-mono text-lg font-semibold">
			{stats.rttAvgMs.toFixed(2)}
			<span class="text-xs font-normal" style="color: var(--color-fg-muted);">ms</span>
		</div>
		{#if stats.rttMdevMs > 0}
			<div class="text-xs" style="color: var(--color-fg-muted);">
				± {stats.rttMdevMs.toFixed(2)}
			</div>
		{/if}
	</div>
	<div>
		<div class="text-[10px] tracking-wider uppercase" style="color: var(--color-fg-subtle);">
			RTT max
		</div>
		<div class="font-mono text-lg font-semibold">
			{stats.rttMaxMs.toFixed(2)}
			<span class="text-xs font-normal" style="color: var(--color-fg-muted);">ms</span>
		</div>
	</div>
</div>
{#if stats.target || stats.source}
	<div
		class="border-t px-4 py-2 font-mono text-xs"
		style="border-color: var(--color-border); color: var(--color-fg-muted);"
	>
		{#if stats.target}<span>target <span style="color: var(--color-fg);">{stats.target}</span></span
			>{/if}
		{#if stats.source}<span class="ml-3"
				>source <span style="color: var(--color-fg);">{stats.source}</span></span
			>{/if}
	</div>
{/if}
