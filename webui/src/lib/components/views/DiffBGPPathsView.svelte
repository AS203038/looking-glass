<script lang="ts">
	import type { Pb } from '$lib/grpc';
	import { computeVirtualWindow } from '$lib/virtual';
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

	function pathBaseKey(p: Pb.BGPPath): string {
		return `${p.prefix}|${p.nexthop}|${p.peerIp || p.peerAsn}`;
	}

	function listEqual(a?: string[], b?: string[]): boolean {
		const x = a ?? [];
		const y = b ?? [];
		if (x.length !== y.length) return false;
		return x.every((v, i) => v === y[i]);
	}

	function formatPeer(p?: Pb.BGPPath): string {
		if (!p) return '—';
		const parts: string[] = [];
		if (p.peerAsn) parts.push(`AS${p.peerAsn}`);
		if (p.peerIp) parts.push(p.peerIp);
		return parts.join(' ') || '—';
	}

	function formatCommunities(p?: Pb.BGPPath): string[] {
		if (!p) return [];
		return [...(p.communities ?? []), ...(p.largeCommunities ?? [])];
	}

	const rows = $derived.by(() => {
		const map = new Map<string, DiffPath>();

		const oldCounts = new Map<string, number>();
		for (const p of oldPaths.paths || []) {
			const base = pathBaseKey(p);
			const idx = oldCounts.get(base) ?? 0;
			oldCounts.set(base, idx + 1);
			const k = `${base}#${idx}`;
			map.set(k, { key: k, old: p, status: 'removed' });
		}

		const newCounts = new Map<string, number>();
		for (const p of newPaths.paths || []) {
			const base = pathBaseKey(p);
			const idx = newCounts.get(base) ?? 0;
			newCounts.set(base, idx + 1);
			const k = `${base}#${idx}`;
			if (map.has(k)) {
				const existing = map.get(k)!;
				existing.new = p;

				const o = existing.old!;
				const changed =
					o.best !== p.best ||
					o.localPref !== p.localPref ||
					o.med !== p.med ||
					o.origin !== p.origin ||
					(o.asPath ?? []).join(' ') !== (p.asPath ?? []).join(' ') ||
					!listEqual(o.communities, p.communities) ||
					!listEqual(o.largeCommunities, p.largeCommunities);

				existing.status = changed ? 'changed' : 'unchanged';
			} else {
				map.set(k, { key: k, new: p, status: 'added' });
			}
		}

		return Array.from(map.values()).sort((a, b) => {
			if (a.new?.best && !b.new?.best) return -1;
			if (!a.new?.best && b.new?.best) return 1;
			if (a.old?.best && !b.old?.best) return -1;
			if (!a.old?.best && b.old?.best) return 1;
			return a.key.localeCompare(b.key);
		});
	});

	const ROW_PX = 28;
	let scrollEl: HTMLDivElement | null = $state(null);
	let scrollTop = $state(0);
	let clientHeight = $state(400);

	const win = $derived(
		computeVirtualWindow(rows.length, scrollTop, clientHeight, { rowPx: ROW_PX })
	);
	const visibleRows = $derived(win.virtualized ? rows.slice(win.startIdx, win.endIdx) : rows);

	$effect(() => {
		void rows;
		if (scrollEl) scrollEl.scrollTop = 0;
		scrollTop = 0;
	});
</script>

<div
	class="flex-1 overflow-auto bg-(--color-bg)"
	bind:this={scrollEl}
	onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
	bind:clientHeight
>
	<table class="w-full font-mono text-xs tabular-nums">
		<thead
			class="sticky top-0 z-10"
			style="background-color: var(--color-bg-inset); color: var(--color-fg-subtle);"
		>
			<tr class="text-left">
				<th class="w-8 px-2 py-2" aria-label="Best path"></th>
				<th class="w-6 px-2 py-2 font-normal">Diff</th>
				<th class="px-3 py-2 font-normal">Prefix</th>
				<th class="px-3 py-2 font-normal">Next Hop</th>
				<th class="px-3 py-2 text-right font-normal">Metric</th>
				<th class="px-3 py-2 text-right font-normal">LocPrf</th>
				<th class="px-3 py-2 font-normal">AS Path</th>
				<th class="px-3 py-2 font-normal">Peer</th>
				<th class="px-3 py-2 font-normal">Communities</th>
			</tr>
		</thead>
		<tbody>
			{#if win.padTopPx > 0}
				<tr aria-hidden="true" style="height: {win.padTopPx}px;"><td colspan="9"></td></tr>
			{/if}
			{#each visibleRows as r (r.key)}
				{@const row = r.new || r.old}
				{@const oldComms = formatCommunities(r.old)}
				{@const newComms = formatCommunities(r.new)}
				{@const commsChanged = r.status === 'changed' && !listEqual(oldComms, newComms)}
				<tr
					class="border-t"
					style="border-color: var(--color-border); height: {win.virtualized &&
					r.status !== 'changed'
						? ROW_PX + 'px'
						: 'auto'}; background-color: {r.status === 'added'
						? 'color-mix(in oklab, var(--color-success) 10%, transparent)'
						: r.status === 'removed'
							? 'color-mix(in oklab, var(--color-danger) 10%, transparent)'
							: r.status === 'changed'
								? 'color-mix(in oklab, var(--color-warning) 10%, transparent)'
								: 'transparent'};"
				>
					<td class="px-2 py-1.5 align-top whitespace-nowrap">
						{#if r.status === 'changed' && r.old?.best !== r.new?.best}
							{#if r.new?.best}
								<span class="font-bold text-(--color-success)" title="Became best path">▲</span>
							{:else}
								<span class="line-through opacity-50" title="Lost best path">▼</span>
							{/if}
						{:else if row?.best}
							<span title="Best path">
								<Check size={14} class="mt-0.5 text-(--color-success)" />
							</span>
						{/if}
					</td>
					<td class="px-2 py-1.5 align-top font-bold">
						{#if r.status === 'added'}<span style="color: var(--color-success)">+</span>
						{:else if r.status === 'removed'}<span style="color: var(--color-danger)">-</span>
						{:else if r.status === 'changed'}<span style="color: var(--color-warning)">~</span>
						{/if}
					</td>
					<td class="px-3 py-1.5 align-top whitespace-nowrap">{row?.prefix || '—'}</td>
					<td class="px-3 py-1.5 align-top whitespace-nowrap">{row?.nexthop || '—'}</td>

					<td class="px-3 py-1.5 text-right align-top whitespace-nowrap">
						{#if r.status === 'changed' && r.old?.med !== r.new?.med}
							<span class="mr-1 line-through opacity-50">{r.old?.med ?? '—'}</span>
							<span class="font-bold text-(--color-success)">{r.new?.med ?? '—'}</span>
						{:else}
							{row?.med ?? '—'}
						{/if}
					</td>

					<td class="px-3 py-1.5 text-right align-top whitespace-nowrap">
						{#if r.status === 'changed' && r.old?.localPref !== r.new?.localPref}
							<span class="mr-1 line-through opacity-50">{r.old?.localPref ?? '—'}</span>
							<span class="font-bold text-(--color-success)">{r.new?.localPref ?? '—'}</span>
						{:else}
							{row?.localPref ?? '—'}
						{/if}
					</td>

					<td class="min-w-0 px-3 py-1.5 align-top break-words">
						{#if r.status === 'changed' && (r.old?.asPath ?? []).join(' ') !== (r.new?.asPath ?? []).join(' ')}
							<div class="line-through opacity-50">{r.old?.asPath?.join(' ') || '—'}</div>
							<div class="font-bold text-(--color-success)">{r.new?.asPath?.join(' ') || '—'}</div>
						{:else}
							{row?.asPath?.join(' ') || '—'}
						{/if}
					</td>

					<td class="px-3 py-1.5 align-top whitespace-nowrap" style="color: var(--color-fg-muted);">
						{formatPeer(row)}
					</td>

					<td class="min-w-0 px-3 py-1.5 align-top">
						{#if commsChanged}
							<div class="line-through opacity-50">{oldComms.join(' ') || '—'}</div>
							<div class="font-bold text-(--color-success)">{newComms.join(' ') || '—'}</div>
						{:else}
							{@const comms = formatCommunities(row)}
							{#if comms.length === 0}
								<span style="color: var(--color-fg-subtle);">—</span>
							{:else}
								<span class="inline-flex flex-wrap gap-1">
									{#each comms as c, ci (ci)}
										<span class="lg-badge !px-1.5 !py-0 text-[10px]">{c}</span>
									{/each}
								</span>
							{/if}
						{/if}
					</td>
				</tr>
			{/each}
			{#if win.padBotPx > 0}
				<tr aria-hidden="true" style="height: {win.padBotPx}px;"><td colspan="9"></td></tr>
			{/if}
		</tbody>
	</table>
</div>
