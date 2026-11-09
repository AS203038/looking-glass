<script lang="ts">
	import { float32 } from '$lib/format';
	import type { Pb } from '$lib/grpc';

	interface Props {
		oldStats: Pb.PingStats;
		newStats: Pb.PingStats;
	}
	let { oldStats, newStats }: Props = $props();

	function diffStr(oldV: number | string, newV: number | string): string {
		if (oldV === newV) return `${newV}`;
		return `${oldV} ➔ ${newV}`;
	}
</script>

<div class="grid grid-cols-2 gap-4 p-4 font-mono text-sm sm:grid-cols-4 sm:gap-6">
	<div class="flex flex-col gap-1">
		<span style="color: var(--color-fg-muted);">Loss</span>
		<div class="flex items-center gap-2">
			{#if oldStats.lossPct !== newStats.lossPct}
				<span class="line-through opacity-50">{oldStats.lossPct}%</span>
			{/if}
			<span
				class="font-semibold"
				style="color: {newStats.lossPct > 0 ? 'var(--color-danger)' : 'var(--color-success)'}"
			>
				{newStats.lossPct}%
			</span>
		</div>
	</div>
	<div class="flex flex-col gap-1">
		<span style="color: var(--color-fg-muted);">Sent / Recv</span>
		<span
			>{diffStr(oldStats.packetsSent, newStats.packetsSent)} / {diffStr(
				oldStats.packetsReceived,
				newStats.packetsReceived
			)}</span
		>
	</div>
	<div class="flex flex-col gap-1">
		<span style="color: var(--color-fg-muted);">Min / Avg / Max</span>
		<span>
			{oldStats.rttMinMs === newStats.rttMinMs
				? float32(newStats.rttMinMs)
				: `${float32(oldStats.rttMinMs)}➔${float32(newStats.rttMinMs)}`}/{oldStats.rttAvgMs ===
			newStats.rttAvgMs
				? float32(newStats.rttAvgMs)
				: `${float32(oldStats.rttAvgMs)}➔${float32(newStats.rttAvgMs)}`}/{oldStats.rttMaxMs ===
			newStats.rttMaxMs
				? float32(newStats.rttMaxMs)
				: `${float32(oldStats.rttMaxMs)}➔${float32(newStats.rttMaxMs)}`}
		</span>
	</div>
	<div class="flex flex-col gap-1">
		<span style="color: var(--color-fg-muted);">Target</span>
		<span class="truncate" title={diffStr(oldStats.target, newStats.target)}
			>{diffStr(oldStats.target, newStats.target)}</span
		>
	</div>
</div>
