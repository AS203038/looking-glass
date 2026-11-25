<script lang="ts">
	import type { Pb } from '$lib/grpc';
	import { float32 } from '$lib/format';
	import { pivotQuery } from '$lib/stores/query';

	interface Props {
		tp: Pb.TracerouteParsed;
	}
	let { tp }: Props = $props();

	function distinctResponders(probes: Pb.TracerouteProbe[]): Pb.TracerouteProbe[] {
		const seen = new Set<string>();
		const out: Pb.TracerouteProbe[] = [];
		for (const p of probes) {
			if (!p.ip && !p.hostname) continue;
			const key = `${p.ip}|${p.hostname}|${p.asn}`;
			if (seen.has(key)) continue;
			seen.add(key);
			out.push(p);
		}
		return out;
	}
</script>

<div class="flex-1 overflow-auto">
	<table class="w-full font-mono text-xs tabular-nums">
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
				{@const responders = distinctResponders(hop.probes)}
				{@const allTimedOut = hop.probes.every((p) => !p.ip && !p.hostname && p.rttMs <= 0)}
				<tr
					class="border-t"
					style="border-color: var(--color-border);"
					class:opacity-50={allTimedOut}
				>
					<td class="px-3 py-1.5 align-top" style="color: var(--color-fg-muted);">{hop.ttl}</td>
					<td class="px-3 py-1.5 align-top">
						{#if responders.length > 0}
							<div class="flex flex-col">
								{#each responders as r, ri (ri)}
									<span class="flex items-center gap-1.5">
										<span>{r.hostname || '—'}</span>
										{#if r.asn > 0}
											<span class="lg-badge !px-1 !py-0 text-[10px]">AS{r.asn}</span>
										{/if}
									</span>
								{/each}
							</div>
						{:else}
							<span style="color: var(--color-fg-subtle);">—</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 align-top">
						{#if responders.length > 0}
							<div class="flex flex-col">
								{#each responders as r, ri (ri)}
									{#if r.ip}
										<button
											type="button"
											class="w-fit text-left hover:underline"
											title={`Pivot: ping ${r.ip}`}
											onclick={() => pivotQuery({ command: 'ping', parameter: r.ip })}
										>
											{r.ip}
										</button>
									{:else}
										<span style="color: var(--color-fg-subtle);">*</span>
									{/if}
								{/each}
							</div>
						{:else}
							<span style="color: var(--color-fg-subtle);">*</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 text-right align-top" style="color: var(--color-fg-muted);">
						{#each hop.probes as p, j (j)}
							{#if p.ip || p.rttMs > 0}
								<span class="ml-2">{float32(p.rttMs)} ms</span>
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
		{#if tp.source}<span class="ml-3">
				source <span style="color: var(--color-fg);">{tp.source}</span>
			</span>{/if}
	</div>
{/if}
