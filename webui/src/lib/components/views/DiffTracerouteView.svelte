<script lang="ts">
	import type { Pb } from '$lib/grpc';

	interface Props {
		oldTp: Pb.TracerouteParsed;
		newTp: Pb.TracerouteParsed;
	}
	let { oldTp, newTp }: Props = $props();

	interface DiffHop {
		ttl: number;
		old?: Pb.TracerouteHop;
		new?: Pb.TracerouteHop;
		status: 'added' | 'removed' | 'changed' | 'unchanged';
	}

	const maxTtl = $derived(Math.max(
		oldTp.hops.length ? oldTp.hops[oldTp.hops.length - 1].ttl : 0,
		newTp.hops.length ? newTp.hops[newTp.hops.length - 1].ttl : 0
	));

	const rows = $derived.by(() => {
		const oldMap = new Map(oldTp.hops.map(h => [h.ttl, h]));
		const newMap = new Map(newTp.hops.map(h => [h.ttl, h]));
		const arr: DiffHop[] = [];
		
		for (let ttl = 1; ttl <= maxTtl; ttl++) {
			const o = oldMap.get(ttl);
			const n = newMap.get(ttl);
			if (!o && !n) continue;
			
			if (o && !n) arr.push({ ttl, old: o, status: 'removed' });
			else if (!o && n) arr.push({ ttl, new: n, status: 'added' });
			else if (o && n) {
				// To check if changed, we can stringify their probes simply
				const oStr = o.probes.map(p => `${p.ip}-${p.rttMs}`).join('|');
				const nStr = n.probes.map(p => `${p.ip}-${p.rttMs}`).join('|');
				arr.push({ ttl, old: o, new: n, status: oStr === nStr ? 'unchanged' : 'changed' });
			}
		}
		return arr;
	});
</script>

<div class="flex-1 overflow-auto bg-(--color-bg-base)">
	<table class="w-full font-mono text-xs">
		<thead class="sticky top-0" style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);">
			<tr class="text-left">
				<th class="w-8 px-3 py-2 font-normal">#</th>
				<th class="px-3 py-2 font-normal">Diff</th>
				<th class="px-3 py-2 font-normal">Host (Old)</th>
				<th class="px-3 py-2 font-normal">Host (New)</th>
				<th class="px-3 py-2 font-normal text-right">RTT</th>
			</tr>
		</thead>
		<tbody>
			{#each rows as r (r.ttl)}
				<tr class="border-t" style="border-color: var(--color-border); background-color: {r.status === 'added' ? 'color-mix(in oklab, var(--color-success) 10%, transparent)' : r.status === 'removed' ? 'color-mix(in oklab, var(--color-danger) 10%, transparent)' : r.status === 'changed' ? 'color-mix(in oklab, var(--color-warning) 10%, transparent)' : 'transparent'};">
					<td class="px-3 py-1.5 whitespace-nowrap text-right" style="color: var(--color-fg-muted);">{r.ttl}</td>
					<td class="px-3 py-1.5 whitespace-nowrap font-bold">
						{#if r.status === 'added'}<span style="color: var(--color-success)">+</span>
						{:else if r.status === 'removed'}<span style="color: var(--color-danger)">-</span>
						{:else if r.status === 'changed'}<span style="color: var(--color-warning)">~</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 whitespace-nowrap opacity-70">
						{#if r.old}
							<div class="flex flex-col">
								{#each r.old.probes as p}
									{#if p.ip}
										<span>{p.hostname && p.hostname !== p.ip ? p.hostname : p.ip}</span>
									{:else}
										<span>*</span>
									{/if}
								{/each}
							</div>
						{/if}
					</td>
					<td class="px-3 py-1.5 whitespace-nowrap">
						{#if r.new}
							<div class="flex flex-col">
								{#each r.new.probes as p}
									{#if p.ip}
										<span>{p.hostname && p.hostname !== p.ip ? p.hostname : p.ip}</span>
									{:else}
										<span>*</span>
									{/if}
								{/each}
							</div>
						{/if}
					</td>
					<td class="px-3 py-1.5 whitespace-nowrap text-right">
						{#if r.status === 'changed'}
							<div class="flex flex-col items-end opacity-50 line-through">
								{#each r.old?.probes || [] as p}
									<span>{p.ip ? p.rttMs + ' ms' : '*'}</span>
								{/each}
							</div>
							<div class="flex flex-col items-end text-green-500">
								{#each r.new?.probes || [] as p}
									<span>{p.ip ? p.rttMs + ' ms' : '*'}</span>
								{/each}
							</div>
						{:else}
							<div class="flex flex-col items-end">
								{#each (r.new || r.old)?.probes || [] as p}
									<span>{p.ip ? p.rttMs + ' ms' : '*'}</span>
								{/each}
							</div>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
