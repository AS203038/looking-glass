<script lang="ts">
	import { fade } from 'svelte/transition';
	import type { ExecResult } from '$lib/stores/query';
	import Loader from './Loader.svelte';
	import { now, relativeTime, absoluteTime, isoTime } from '$lib/time';
	import Copy from '@lucide/svelte/icons/copy';
	import Download from '@lucide/svelte/icons/download';
	import Search from '@lucide/svelte/icons/search';
	import CircleX from '@lucide/svelte/icons/circle-x';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Clock from '@lucide/svelte/icons/clock';
	import MapPin from '@lucide/svelte/icons/map-pin';
	import Table2 from '@lucide/svelte/icons/table-2';
	import FileText from '@lucide/svelte/icons/file-text';
	import PingView from './views/PingView.svelte';
	import TracerouteView from './views/TracerouteView.svelte';
	import BGPPathsView from './views/BGPPathsView.svelte';
	import BGPSummaryView from './views/BGPSummaryView.svelte';
	import DiffPingView from './views/DiffPingView.svelte';
	import DiffTracerouteView from './views/DiffTracerouteView.svelte';
	import DiffBGPPathsView from './views/DiffBGPPathsView.svelte';
	import DiffBGPSummaryView from './views/DiffBGPSummaryView.svelte';
	import { history } from '$lib/stores/history';
	import FileDiff from '@lucide/svelte/icons/file-diff';
	import DiffWorker from '$lib/workers/diff.worker.ts?worker';
	import { onDestroy } from 'svelte';

	interface Props {
		result: ExecResult;
	}
	let { result }: Props = $props();

	let copied = $state(false);
	let search = $state('');
	let windowLimit = $state(500);
	const WINDOW_STEP = 500;
	let nowDate = $state(new Date());
	now.subscribe((d) => (nowDate = d));

	let viewMode = $state<'structured' | 'raw'>('structured');
	const canStructured = $derived(result.parsed != null);

	let compareWith = $state<ExecResult | null>(null);
	let diffViewMode = $state<'diff' | 'raw-split' | 'structured-split' | 'structured-diff'>(
		'structured-diff'
	);

	const pastRuns = $derived.by(() => {
		if (!result || !result.timestamp) return [];
		const matches = [];
		for (const h of $history) {
			if (h.command === result.queryCommand && h.parameter === result.queryParameter) {
				const pastRes = h.results[result.routerId];
				if (!pastRes || !pastRes.timestamp) continue;
				// Skip if it's the exact same execution instance
				if (pastRes.timestamp.getTime() === result.timestamp.getTime()) continue;

				if (pastRes.status === 'done') {
					matches.push({ timestamp: h.timestamp, result: pastRes });
				}
			}
		}
		return matches.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
	});

	$effect(() => {
		void result.timestamp;
		compareWith = null;
	});

	const decoder = new TextDecoder('utf-8', { fatal: false });

	function decode(bytes: Uint8Array | null | undefined): string {
		if (!bytes) return '';
		return decoder.decode(bytes);
	}

	interface DiffLine {
		value: string;
		added?: boolean;
		removed?: boolean;
	}
	let diffLinesArr = $state<DiffLine[]>([]);
	let diffLoading = $state(false);
	let diffError = $state<string | null>(null);
	let diffWorker: Worker | null = null;

	$effect(() => {
		if (!compareWith || diffViewMode !== 'diff') return;

		const oldStr = decode(compareWith.bytes);
		const newStr = decode(result.bytes);

		diffLoading = true;
		diffError = null;
		diffLinesArr = [];

		if (!diffWorker) {
			diffWorker = new DiffWorker();
		}

		diffWorker.onmessage = (e) => {
			if (e.data.type === 'success') {
				diffLinesArr = e.data.lines;
			} else {
				diffError = e.data.error;
			}
			diffLoading = false;
		};

		diffWorker.postMessage({ oldStr, newStr });
	});

	onDestroy(() => {
		if (diffWorker) {
			diffWorker.terminate();
			diffWorker = null;
		}
	});

	let diffWindowLimit = $state(500);
	const visibleDiffLines = $derived(diffLinesArr.slice(0, diffWindowLimit));
	const hasMoreDiff = $derived(diffLinesArr.length > diffWindowLimit);

	const lines = $derived.by(() => {
		if (!result.bytes) return [] as string[];
		const text = new TextDecoder('utf-8', { fatal: false }).decode(result.bytes);
		return text.replace(/\n+$/, '').split('\n');
	});

	const filteredLines = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return lines;
		return lines.filter((l) => l.toLowerCase().includes(q));
	});

	const visibleLines = $derived(filteredLines.slice(0, windowLimit));
	const hasMore = $derived(filteredLines.length > windowLimit);

	const compareLines = $derived.by(() => {
		if (!compareWith || !compareWith.bytes) return [] as string[];
		const text = new TextDecoder('utf-8', { fatal: false }).decode(compareWith.bytes);
		return text.replace(/\n+$/, '').split('\n');
	});

	const filteredCompareLines = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return compareLines;
		return compareLines.filter((l) => l.toLowerCase().includes(q));
	});

	const visibleCompareLines = $derived(filteredCompareLines.slice(0, windowLimit));
	const hasMoreCompare = $derived(filteredCompareLines.length > windowLimit);

	const parserLabel = $derived.by(() => {
		switch (result.parserKind) {
			case 1:
				return 'textfsm';
			case 2:
				return 'native_json';
			case 3:
				return 'builtin';
			default:
				return '';
		}
	});

	async function copyAll() {
		if (!result.bytes) return;
		const text = new TextDecoder().decode(result.bytes);
		try {
			await navigator.clipboard.writeText(text);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch (err) {
			console.error('Clipboard write failed:', err);
		}
	}

	function download() {
		if (!result.bytes) return;
		const blob = new Blob([result.bytes as unknown as BlobPart], { type: 'text/plain' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		const safeCmd = result.resolvedCommand;
		a.href = url;
		a.download = `${result.routerName}-${safeCmd}-${new Date().toISOString().replace(/[:.]/g, '-')}.txt`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<article
	class="lg-card flex flex-col overflow-hidden"
	style="max-height: calc(var(--sheet-body-height) - 2rem);"
	in:fade={{ duration: 150 }}
>
	<header
		class="flex flex-wrap items-center gap-2 border-b px-4 py-3"
		style="border-color: var(--color-border);"
	>
		<div class="flex min-w-0 flex-1 flex-col">
			<span class="truncate text-sm font-semibold">{result.routerName}</span>
			{#if result.routerLocation}
				<span class="flex items-center gap-1 text-xs" style="color: var(--color-fg-muted);">
					<MapPin size={11} class="opacity-60" />
					<span class="capitalize">{result.routerLocation}</span>
				</span>
			{/if}
		</div>

		{#if result.status === 'pending' || result.status === 'running'}
			<span class="lg-badge">
				{result.status === 'pending' ? 'queued' : 'running…'}
			</span>
		{:else if result.status === 'done'}
			<span class="lg-badge lg-badge-success">
				<CircleCheck size={12} />
				done
			</span>
		{:else}
			<span class="lg-badge lg-badge-danger">
				<CircleX size={12} />
				error
			</span>
		{/if}

		<div class="ml-auto flex items-center gap-2">
			{#if pastRuns.length > 0}
				<select
					class="lg-input h-7 min-w-[100px] px-2 py-1 text-xs"
					style="background-color: var(--color-bg-inset);"
					value={compareWith && compareWith.timestamp
						? String(compareWith.timestamp.getTime())
						: ''}
					onchange={(e) => {
						const val = e.currentTarget.value;
						if (!val) {
							compareWith = null;
						} else {
							const ts = parseInt(val);
							compareWith =
								pastRuns.find((r) => r.result.timestamp && r.result.timestamp.getTime() === ts)
									?.result || null;
						}
					}}
				>
					<option value="">Compare…</option>
					{#each pastRuns as run}
						<option value={run.result.timestamp ? String(run.result.timestamp.getTime()) : ''}
							>vs {relativeTime(run.timestamp, nowDate)}</option
						>
					{/each}
				</select>
			{/if}

			{#if result.cached}
				<span
					class="lg-badge lg-badge-info tracking-wider uppercase"
					style="font-size: 9px; padding: 0.125rem 0.375rem; height: max-content;">Cached</span
				>
			{/if}

			{#if result.timestamp}
				<time
					class="flex items-center gap-1 font-mono text-xs"
					style="color: var(--color-fg-subtle);"
					datetime={isoTime(result.timestamp)}
					title={absoluteTime(result.timestamp)}
				>
					<Clock size={11} class="opacity-60" />
					{relativeTime(result.timestamp, nowDate)}
				</time>
			{/if}
		</div>
	</header>

	{#if result.status === 'pending' || result.status === 'running'}
		<div class="flex justify-center p-8"><Loader label={result.status} size="sm" /></div>
	{:else if result.status === 'error'}
		<div
			class="flex items-start gap-2 p-4 text-sm"
			style="color: var(--color-danger); background-color: color-mix(in oklab, var(--color-danger) 8%, transparent);"
		>
			<CircleX size={16} class="mt-0.5 shrink-0" />
			<span class="font-mono break-words whitespace-pre-wrap">{result.error}</span>
		</div>
	{:else}
		<div
			class="flex flex-wrap items-center gap-2 border-b px-3 py-2"
			style="border-color: var(--color-border);"
		>
			{#if compareWith}
				<div
					class="inline-flex overflow-hidden rounded-md border"
					style="border-color: var(--color-border);"
				>
					<button
						type="button"
						class="flex items-center gap-1 px-2 py-1 text-xs"
						class:font-semibold={diffViewMode === 'diff'}
						style:background-color={diffViewMode === 'diff'
							? 'var(--color-bg-inset)'
							: 'transparent'}
						onclick={() => (diffViewMode = 'diff')}
						title="Unified Diff"
					>
						<FileDiff size={12} />
						Diff
					</button>
					<button
						type="button"
						class="flex items-center gap-1 px-2 py-1 text-xs"
						class:font-semibold={diffViewMode === 'raw-split'}
						style:background-color={diffViewMode === 'raw-split'
							? 'var(--color-bg-inset)'
							: 'transparent'}
						onclick={() => (diffViewMode = 'raw-split')}
						title="Raw Side-by-Side"
					>
						<FileText size={12} />
						Raw Split
					</button>
					{#if canStructured || compareWith.parsed}
						<button
							type="button"
							class="flex items-center gap-1 px-2 py-1 text-xs"
							class:font-semibold={diffViewMode === 'structured-diff'}
							style:background-color={diffViewMode === 'structured-diff'
								? 'var(--color-bg-inset)'
								: 'transparent'}
							onclick={() => (diffViewMode = 'structured-diff')}
							title="Structured Diff"
						>
							<Table2 size={12} />
							Struct Diff
						</button>
						<button
							type="button"
							class="flex items-center gap-1 px-2 py-1 text-xs"
							class:font-semibold={diffViewMode === 'structured-split'}
							style:background-color={diffViewMode === 'structured-split'
								? 'var(--color-bg-inset)'
								: 'transparent'}
							onclick={() => (diffViewMode = 'structured-split')}
							title="Structured Side-by-Side"
						>
							<Table2 size={12} />
							Struct Split
						</button>
					{/if}
				</div>
			{:else}
				{#if canStructured}
					<div
						class="inline-flex overflow-hidden rounded-md border"
						style="border-color: var(--color-border);"
					>
						<button
							type="button"
							class="flex items-center gap-1 px-2 py-1 text-xs"
							class:font-semibold={viewMode === 'structured'}
							style:background-color={viewMode === 'structured'
								? 'var(--color-bg-inset)'
								: 'transparent'}
							onclick={() => (viewMode = 'structured')}
							title="Structured view"
						>
							<Table2 size={12} />
							Structured
						</button>
						<button
							type="button"
							class="flex items-center gap-1 px-2 py-1 text-xs"
							class:font-semibold={viewMode === 'raw'}
							style:background-color={viewMode === 'raw' ? 'var(--color-bg-inset)' : 'transparent'}
							onclick={() => (viewMode = 'raw')}
							title="Raw output"
						>
							<FileText size={12} />
							Raw
						</button>
					</div>
					{#if parserLabel}
						<span
							class="font-mono text-[10px]"
							style="color: var(--color-fg-subtle);"
							title="Parser provenance">via {parserLabel}</span
						>
					{/if}
				{:else if parserLabel}
					<span
						class="font-mono text-[10px]"
						style="color: var(--color-fg-subtle);"
						title="Parser attempted but produced no structured view"
						>parser: {parserLabel} (failed)</span
					>
				{/if}

				{#if !canStructured || viewMode === 'raw'}
					<div class="relative flex-1 sm:max-w-xs">
						<Search
							size={14}
							class="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 opacity-60"
						/>
						<input
							type="search"
							class="lg-input py-1.5 pl-8 text-xs"
							placeholder="Filter lines…"
							bind:value={search}
						/>
					</div>
					<span class="ml-auto font-mono text-xs" style="color: var(--color-fg-subtle);">
						{filteredLines.length} / {lines.length} lines
					</span>
				{:else}
					<span class="ml-auto"></span>
				{/if}

				<button
					type="button"
					class="lg-btn lg-btn-outline h-8 !px-2.5 text-xs"
					onclick={copyAll}
					title="Copy full output to clipboard"
				>
					<Copy size={12} />
					{copied ? 'Copied' : 'Copy'}
				</button>
				<button
					type="button"
					class="lg-btn lg-btn-outline h-8 !px-2.5 text-xs"
					onclick={download}
					title="Download full output as .txt"
				>
					<Download size={12} />
					Download
				</button>
			{/if}
		</div>

		{#if compareWith}
			{#if diffViewMode === 'diff'}
				{#if diffLoading}
					<div class="flex justify-center p-8"><Loader label="Computing diff…" size="sm" /></div>
				{:else if diffError}
					<div class="p-4 text-sm" style="color: var(--color-danger);">
						Failed to compute diff: {diffError}
					</div>
				{:else}
					<div
						class="flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
						style="background-color: var(--color-bg-inset); color: var(--color-fg-base);"
					>
						{#each visibleDiffLines as part}
							<div
								style="color: {part.added
									? 'var(--color-success)'
									: part.removed
										? 'var(--color-danger)'
										: 'inherit'}; background-color: {part.added
									? 'color-mix(in oklab, var(--color-success) 15%, transparent)'
									: part.removed
										? 'color-mix(in oklab, var(--color-danger) 15%, transparent)'
										: 'transparent'};"
							>
								{part.value}
							</div>
						{/each}
					</div>
					{#if hasMoreDiff}
						<div
							class="flex items-center justify-between border-t px-4 py-2 text-xs"
							style="border-color: var(--color-border); color: var(--color-fg-muted); background-color: var(--color-bg-base);"
						>
							<span>Showing {diffWindowLimit.toLocaleString()} diff lines</span>
							<button
								type="button"
								class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
								onclick={() => (diffWindowLimit += WINDOW_STEP)}>Show more</button
							>
						</div>
					{/if}
				{/if}
			{:else if diffViewMode === 'raw-split'}
				<div
					class="grid flex-1 grid-cols-2 divide-x overflow-hidden"
					style="border-color: var(--color-border); background-color: var(--color-bg-inset);"
				>
					<div class="flex h-full min-w-0 flex-col">
						<div
							class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
							style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
						>
							Older
						</div>
						<pre
							class="flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
							style="color: var(--color-fg-base);"><code>{visibleCompareLines.join('\n')}</code
							></pre>
					</div>
					<div class="flex h-full min-w-0 flex-col">
						<div
							class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
							style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
						>
							Newer
						</div>
						<pre
							class="flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
							style="color: var(--color-fg-base);"><code>{visibleLines.join('\n')}</code></pre>
					</div>
				</div>
				{#if hasMore || hasMoreCompare}
					<div
						class="flex items-center justify-between border-t px-4 py-2 text-xs"
						style="border-color: var(--color-border); color: var(--color-fg-muted); background-color: var(--color-bg-base);"
					>
						<span>
							Showing {windowLimit.toLocaleString()} lines
						</span>
						<button
							type="button"
							class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
							onclick={() => (windowLimit += WINDOW_STEP)}
						>
							Show more
						</button>
					</div>
				{/if}
			{:else if diffViewMode === 'structured-diff'}
				{#if compareWith.parsed && result.parsed && compareWith.parsed.kind === result.parsed.kind}
					<div
						class="flex min-h-0 flex-1 flex-col border-b"
						style="border-color: var(--color-border);"
					>
						{#if result.parsed.kind === 'ping' && compareWith.parsed.kind === 'ping'}
							<DiffPingView oldStats={compareWith.parsed.data} newStats={result.parsed.data} />
						{:else if result.parsed.kind === 'traceroute' && compareWith.parsed.kind === 'traceroute'}
							<DiffTracerouteView oldTp={compareWith.parsed.data} newTp={result.parsed.data} />
						{:else if result.parsed.kind === 'bgp_summary' && compareWith.parsed.kind === 'bgp_summary'}
							<DiffBGPSummaryView
								oldSummary={compareWith.parsed.data}
								newSummary={result.parsed.data}
							/>
						{:else if result.parsed.kind === 'bgp_paths' && compareWith.parsed.kind === 'bgp_paths'}
							<DiffBGPPathsView oldPaths={compareWith.parsed.data} newPaths={result.parsed.data} />
						{/if}
					</div>
				{:else}
					<div class="p-4 text-xs italic" style="color: var(--color-fg-subtle);">
						Structured data format mismatch or unavailable
					</div>
				{/if}
			{:else if diffViewMode === 'structured-split'}
				<div
					class="grid flex-1 grid-cols-1 divide-y overflow-hidden md:grid-cols-2 md:divide-x md:divide-y-0"
					style="border-color: var(--color-border);"
				>
					<div class="flex h-full min-w-0 flex-col bg-(--color-bg-base)">
						<div
							class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
							style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
						>
							Older
						</div>
						<div class="flex min-h-0 flex-1 flex-col">
							{#if compareWith.parsed}
								{#if compareWith.parsed.kind === 'ping'}
									<PingView stats={compareWith.parsed.data} />
								{:else if compareWith.parsed.kind === 'traceroute'}
									<TracerouteView tp={compareWith.parsed.data} />
								{:else if compareWith.parsed.kind === 'bgp_summary'}
									<BGPSummaryView summary={compareWith.parsed.data} />
								{:else if compareWith.parsed.kind === 'bgp_paths'}
									<BGPPathsView paths={compareWith.parsed.data} />
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
					<div class="flex h-full min-w-0 flex-col bg-(--color-bg-base)">
						<div
							class="shrink-0 border-b px-3 py-1.5 text-[10px] font-semibold tracking-wider uppercase"
							style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-fg-muted);"
						>
							Newer
						</div>
						<div class="flex min-h-0 flex-1 flex-col">
							{#if result.parsed}
								{#if result.parsed.kind === 'ping'}
									<PingView stats={result.parsed.data} />
								{:else if result.parsed.kind === 'traceroute'}
									<TracerouteView tp={result.parsed.data} />
								{:else if result.parsed.kind === 'bgp_summary'}
									<BGPSummaryView summary={result.parsed.data} />
								{:else if result.parsed.kind === 'bgp_paths'}
									<BGPPathsView paths={result.parsed.data} />
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
			{/if}
		{:else if canStructured && viewMode === 'structured' && result.parsed}
			<div class="flex min-h-0 flex-1 flex-col">
				{#if result.parsed.kind === 'ping'}
					<PingView stats={result.parsed.data} />
				{:else if result.parsed.kind === 'traceroute'}
					<TracerouteView tp={result.parsed.data} />
				{:else if result.parsed.kind === 'bgp_summary'}
					<BGPSummaryView summary={result.parsed.data} />
				{:else if result.parsed.kind === 'bgp_paths'}
					<BGPPathsView paths={result.parsed.data} />
				{/if}
			</div>
		{:else}
			<pre
				class="flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
				style="background-color: var(--color-bg-inset); color: var(--color-fg);"><code
					>{visibleLines.join('\n')}</code
				></pre>

			{#if hasMore}
				<div
					class="flex items-center justify-between border-t px-4 py-2 text-xs"
					style="border-color: var(--color-border); color: var(--color-fg-muted);"
				>
					<span>
						Showing {windowLimit.toLocaleString()} of {filteredLines.length.toLocaleString()} lines
					</span>
					<button
						type="button"
						class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
						onclick={() => (windowLimit += WINDOW_STEP)}
					>
						Show {Math.min(WINDOW_STEP, filteredLines.length - windowLimit)} more
					</button>
				</div>
			{/if}
		{/if}
	{/if}
</article>
