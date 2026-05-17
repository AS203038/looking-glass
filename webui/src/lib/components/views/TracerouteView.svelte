<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		tp: Pb.TracerouteParsed;
	}
	let { tp }: Props = $props();
</script>

<div class="flex-1 overflow-auto">
	<table class="w-full font-mono text-xs">
		<thead
			class="sticky top-0"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="px-3 py-2 font-normal tracking-wider uppercase">TTL</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">Host</th>
				<th class="px-3 py-2 font-normal tracking-wider uppercase">IP</th>
				<th class="px-3 py-2 text-right font-normal tracking-wider uppercase">RTT</th>
			</tr>
		</thead>
		<tbody>
			{#each tp.hops as hop, i (i)}
				<tr
					class="border-t"
					style="border-color: var(--color-border);"
					class:opacity-50={hop.probes.every((p) => !p.ip)}
				>
					<td class="px-3 py-1.5" style="color: var(--color-fg-muted);">{hop.ttl}</td>
					<td class="px-3 py-1.5">
						{#if hop.probes[0]?.hostname}
							{hop.probes[0].hostname}
						{:else}
							<span style="color: var(--color-fg-subtle);">—</span>
						{/if}
					</td>
					<td class="px-3 py-1.5">
						{#if hop.probes[0]?.ip}
							{hop.probes[0].ip}
						{:else}
							<span style="color: var(--color-fg-subtle);">*</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 text-right" style="color: var(--color-fg-muted);">
						{#each hop.probes as p, j (j)}
							{#if p.rttMs > 0}
								<span class="ml-2">{p.rttMs.toFixed(1)} ms</span>
							{:else}
								<span class="ml-2" style="color: var(--color-fg-subtle);">*</span>
							{/if}
						{/each}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
{#if tp.target || tp.source}
	<div
		class="border-t px-4 py-2 font-mono text-xs"
		style="border-color: var(--color-border); color: var(--color-fg-muted);"
	>
		{#if tp.target}<span>target <span style="color: var(--color-fg);">{tp.target}</span></span>{/if}
		{#if tp.source}<span class="ml-3"
				>source <span style="color: var(--color-fg);">{tp.source}</span></span
			>{/if}
	</div>
{/if}
