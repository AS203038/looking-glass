<script lang="ts">
	import type { Pb } from '$lib/grpc';
	import Check from '@lucide/svelte/icons/check';

	interface Props {
		oldPaths: Pb.BGPPaths;
		newPaths: Pb.BGPPaths;
	}
	let { oldPaths, newPaths }: Props = $props();

	interface DiffPath {
		key: string;
		old?: Pb.BGPPath;
		new?: Pb.BGPPath;
		status: 'added' | 'removed' | 'changed' | 'unchanged';
	}

	// BGP paths lack a strong primary key, so we use prefix + nexthop + peer as heuristic identity.
	function pathKey(p: Pb.BGPPath) {
		return `${p.prefix}|${p.nexthop}|${p.peerIp || p.peerAsn}`;
	}

	const rows = $derived.by(() => {
		const map = new Map<string, DiffPath>();
		for (const p of oldPaths.paths || []) {
			const k = pathKey(p);
			map.set(k, { key: k, old: p, status: 'removed' });
		}
		for (const p of newPaths.paths || []) {
			const k = pathKey(p);
			if (map.has(k)) {
				const existing = map.get(k)!;
				existing.new = p;

				const o = existing.old!;
				const changed =
					o.best !== p.best ||
					o.localPref !== p.localPref ||
					o.med !== p.med ||
					o.origin !== p.origin ||
					o.asPath?.join(' ') !== p.asPath?.join(' ');

				existing.status = changed ? 'changed' : 'unchanged';
			} else {
				map.set(k, { key: k, new: p, status: 'added' });
			}
		}
		// Sort by best path in new, then old, then lexical key
		return Array.from(map.values()).sort((a, b) => {
			if (a.new?.best && !b.new?.best) return -1;
			if (!a.new?.best && b.new?.best) return 1;
			if (a.old?.best && !b.old?.best) return -1;
			if (!a.old?.best && b.old?.best) return 1;
			return a.key.localeCompare(b.key);
		});
	});

	const VIRTUAL_THRESHOLD = 200;
	const ROW_PX = 28;
	let scrollEl: HTMLDivElement | null = $state(null);
	let scrollTop = $state(0);
	let clientHeight = $state(400);

	const virtualized = $derived(rows.length > VIRTUAL_THRESHOLD);
	const startIdx = $derived(virtualized ? Math.max(0, Math.floor(scrollTop / ROW_PX) - 5) : 0);
	const visibleCount = $derived(
		virtualized
			? Math.min(rows.length - startIdx, Math.ceil(clientHeight / ROW_PX) + 10)
			: rows.length
	);
	const endIdx = $derived(startIdx + visibleCount);
	const padTopPx = $derived(virtualized ? startIdx * ROW_PX : 0);
	const padBotPx = $derived(virtualized ? (rows.length - endIdx) * ROW_PX : 0);
	const visibleRows = $derived(rows.slice(startIdx, endIdx));

	$effect(() => {
		void rows;
		if (scrollEl) scrollEl.scrollTop = 0;
	});
</script>

<div
	class="flex-1 overflow-auto bg-(--color-bg-base)"
	bind:this={scrollEl}
	onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
	bind:clientHeight
>
	<table class="w-full font-mono text-xs">
		<thead
			class="sticky top-0 z-10"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="w-6 px-2 py-2"></th>
				<th class="w-6 px-2 py-2">Diff</th>
				<th class="px-3 py-2 font-normal">Next Hop</th>
				<th class="px-3 py-2 text-right font-normal">Metric</th>
				<th class="px-3 py-2 text-right font-normal">LocPrf</th>
				<th class="px-3 py-2 font-normal">AS Path</th>
			</tr>
		</thead>
		<tbody>
			{#if padTopPx > 0}
				<tr style="height: {padTopPx}px;"><td colspan="6"></td></tr>
			{/if}
			{#each visibleRows as r (r.key)}
				<tr
					class="border-t"
					style="border-color: var(--color-border); height: {virtualized
						? ROW_PX + 'px'
						: 'auto'}; background-color: {r.status === 'added'
						? 'color-mix(in oklab, var(--color-success) 10%, transparent)'
						: r.status === 'removed'
							? 'color-mix(in oklab, var(--color-danger) 10%, transparent)'
							: r.status === 'changed'
								? 'color-mix(in oklab, var(--color-warning) 10%, transparent)'
								: 'transparent'};"
				>
					<td class="px-2 py-1.5 align-top">
						{#if (r.new || r.old)?.best}
							<Check size={14} class="mt-0.5 text-(--color-success)" title="Best path" />
						{/if}
					</td>
					<td class="px-2 py-1.5 align-top font-bold">
						{#if r.status === 'added'}<span style="color: var(--color-success)">+</span>
						{:else if r.status === 'removed'}<span style="color: var(--color-danger)">-</span>
						{:else if r.status === 'changed'}<span style="color: var(--color-warning)">~</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 align-top whitespace-nowrap">{(r.new || r.old)?.nexthop}</td>

					<td class="px-3 py-1.5 text-right align-top whitespace-nowrap">
						{#if r.status === 'changed' && r.old?.med !== r.new?.med}
							<span class="mr-1 line-through opacity-50">{r.old?.med ?? '—'}</span>
							<span class="font-bold text-(--color-success)">{r.new?.med ?? '—'}</span>
						{:else}
							{(r.new || r.old)?.med ?? '—'}
						{/if}
					</td>

					<td class="px-3 py-1.5 text-right align-top whitespace-nowrap">
						{#if r.status === 'changed' && r.old?.localPref !== r.new?.localPref}
							<span class="mr-1 line-through opacity-50">{r.old?.localPref ?? '—'}</span>
							<span class="font-bold text-(--color-success)">{r.new?.localPref ?? '—'}</span>
						{:else}
							{(r.new || r.old)?.localPref ?? '—'}
						{/if}
					</td>

					<td class="min-w-0 px-3 py-1.5 align-top break-words">
						{#if r.status === 'changed' && r.old?.asPath?.join(' ') !== r.new?.asPath?.join(' ')}
							<div class="line-through opacity-50">{r.old?.asPath?.join(' ') || '—'}</div>
							<div class="font-bold text-(--color-success)">{r.new?.asPath?.join(' ') || '—'}</div>
						{:else}
							{(r.new || r.old)?.asPath?.join(' ') || '—'}
						{/if}
					</td>
				</tr>
			{/each}
			{#if padBotPx > 0}
				<tr style="height: {padBotPx}px;"><td colspan="6"></td></tr>
			{/if}
		</tbody>
	</table>
</div>
