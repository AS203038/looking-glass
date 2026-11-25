<script lang="ts">
	import type { ParsedPayload } from '$lib/stores/query';
	import PingView from '../views/PingView.svelte';
	import TracerouteView from '../views/TracerouteView.svelte';
	import BGPPathsView from '../views/BGPPathsView.svelte';
	import BGPSummaryView from '../views/BGPSummaryView.svelte';
	import DiffPingView from '../views/DiffPingView.svelte';
	import DiffTracerouteView from '../views/DiffTracerouteView.svelte';
	import DiffBGPPathsView from '../views/DiffBGPPathsView.svelte';
	import DiffBGPSummaryView from '../views/DiffBGPSummaryView.svelte';

	interface Props {
		mode: 'single' | 'diff' | 'split';
		parsed: ParsedPayload | null;
		compareParsed?: ParsedPayload | null;
		routerId: string;
		nextPageToken?: string;
		loadingMore?: boolean;
		onLoadMore?: () => void;
	}

	let {
		mode,
		parsed,
		compareParsed = null,
		routerId,
		nextPageToken = '',
		loadingMore = false,
		onLoadMore
	}: Props = $props();
</script>

{#if mode === 'diff'}
	{#if compareParsed && parsed && compareParsed.kind === parsed.kind}
		<div class="flex min-h-0 flex-1 flex-col border-b" style="border-color: var(--color-border);">
			{#if parsed.kind === 'ping' && compareParsed.kind === 'ping'}
				<DiffPingView oldStats={compareParsed.data} newStats={parsed.data} />
			{:else if parsed.kind === 'traceroute' && compareParsed.kind === 'traceroute'}
				<DiffTracerouteView oldTp={compareParsed.data} newTp={parsed.data} />
			{:else if parsed.kind === 'bgp_summary' && compareParsed.kind === 'bgp_summary'}
				<DiffBGPSummaryView oldSummary={compareParsed.data} newSummary={parsed.data} />
			{:else if parsed.kind === 'bgp_paths' && compareParsed.kind === 'bgp_paths'}
				<DiffBGPPathsView oldPaths={compareParsed.data} newPaths={parsed.data} />
			{/if}
		</div>
	{:else}
		<div class="p-4 text-xs italic" style="color: var(--color-fg-subtle);">
			Structured data format mismatch or unavailable
		</div>
	{/if}
{:else if mode === 'split'}
	<div
		class="grid flex-1 grid-cols-1 divide-y overflow-hidden md:grid-cols-2 md:divide-x md:divide-y-0"
		style="border-color: var(--color-border);"
	>
		<div class="flex h-full min-w-0 flex-col bg-(--color-bg)">
			<div
				class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
				style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
			>
				Older
			</div>
			<div class="flex min-h-0 flex-1 flex-col">
				{#if compareParsed}
					{#if compareParsed.kind === 'ping'}
						<PingView stats={compareParsed.data} />
					{:else if compareParsed.kind === 'traceroute'}
						<TracerouteView tp={compareParsed.data} />
					{:else if compareParsed.kind === 'bgp_summary'}
						<BGPSummaryView summary={compareParsed.data} {routerId} />
					{:else if compareParsed.kind === 'bgp_paths'}
						<BGPPathsView paths={compareParsed.data} />
					{/if}
				{:else}
					<div
						class="flex-1 overflow-auto p-4 text-xs italic"
						style="color: var(--color-fg-subtle);"
					>
						No structured data available
					</div>
				{/if}
			</div>
		</div>
		<div class="flex h-full min-w-0 flex-col bg-(--color-bg)">
			<div
				class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
				style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
			>
				Newer
			</div>
			<div class="flex min-h-0 flex-1 flex-col">
				{#if parsed}
					{#if parsed.kind === 'ping'}
						<PingView stats={parsed.data} />
					{:else if parsed.kind === 'traceroute'}
						<TracerouteView tp={parsed.data} />
					{:else if parsed.kind === 'bgp_summary'}
						<BGPSummaryView summary={parsed.data} {routerId} />
					{:else if parsed.kind === 'bgp_paths'}
						<BGPPathsView paths={parsed.data} {nextPageToken} {loadingMore} {onLoadMore} />
					{/if}
				{:else}
					<div
						class="flex-1 overflow-auto p-4 text-xs italic"
						style="color: var(--color-fg-subtle);"
					>
						No structured data available
					</div>
				{/if}
			</div>
		</div>
	</div>
{:else if parsed}
	<div class="flex min-h-0 flex-1 flex-col">
		{#if parsed.kind === 'ping'}
			<PingView stats={parsed.data} />
		{:else if parsed.kind === 'traceroute'}
			<TracerouteView tp={parsed.data} />
		{:else if parsed.kind === 'bgp_summary'}
			<BGPSummaryView summary={parsed.data} {routerId} />
		{:else if parsed.kind === 'bgp_paths'}
			<BGPPathsView paths={parsed.data} {nextPageToken} {loadingMore} {onLoadMore} />
		{/if}
	</div>
{/if}
