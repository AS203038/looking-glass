<script lang="ts">
	import { fade } from 'svelte/transition';
	import { loadMorePeerRoutes, type ExecResult } from '$lib/stores/query';
	import { Pb } from '$lib/grpc';
	import { history } from '$lib/stores/history';
	import { now, relativeTime, absoluteTime, isoTime } from '$lib/time';
	import { transitionDuration } from '$lib/motion';
	import Loader from './Loader.svelte';
	import ResultToolbar, { type DiffViewMode, type ViewMode } from './result/ResultToolbar.svelte';
	import RawOutputView from './result/RawOutputView.svelte';
	import RawDiffView from './result/RawDiffView.svelte';
	import StructuredView from './result/StructuredView.svelte';
	import CircleX from '@lucide/svelte/icons/circle-x';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Clock from '@lucide/svelte/icons/clock';
	import MapPin from '@lucide/svelte/icons/map-pin';

	interface Props {
		result: ExecResult;
	}
	let { result }: Props = $props();

	const WINDOW_STEP = 500;
	const decoder = new TextDecoder('utf-8', { fatal: false });

	let copied = $state(false);
	let search = $state('');
	let windowLimit = $state(WINDOW_STEP);
	let viewMode = $state<ViewMode>('structured');
	let compareEntryId = $state('');
	let diffViewMode = $state<DiffViewMode>('structured-diff');

	const canStructured = $derived(result.parsed != null);

	const pastRuns = $derived.by(() => {
		if (!result || !result.timestamp) return [];
		const matches: { entryId: string; timestamp: Date; result: ExecResult }[] = [];
		for (const h of $history) {
			if (h.command === result.queryCommand && h.parameter === result.queryParameter) {
				const pastRes = h.results[result.routerId];
				if (!pastRes || !pastRes.timestamp) continue;
				if (pastRes.timestamp.getTime() === result.timestamp.getTime()) continue;
				if (pastRes.status === 'done') {
					matches.push({ entryId: h.id, timestamp: h.timestamp, result: pastRes });
				}
			}
		}
		return matches.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
	});

	const compareWith = $derived(pastRuns.find((r) => r.entryId === compareEntryId)?.result ?? null);

	$effect(() => {
		void result.timestamp;
		void result.routerId;
		compareEntryId = '';
		search = '';
		windowLimit = WINDOW_STEP;
	});

	$effect(() => {
		void compareEntryId;
		windowLimit = WINDOW_STEP;
	});

	function decode(bytes: Uint8Array | null | undefined): string {
		if (!bytes) return '';
		return decoder.decode(bytes);
	}

	function splitLines(bytes: Uint8Array | null | undefined): string[] {
		if (!bytes) return [];
		return decode(bytes).replace(/\n+$/, '').split('\n');
	}

	const lines = $derived(splitLines(result.bytes));
	const filteredLines = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return lines;
		return lines.filter((l) => l.toLowerCase().includes(q));
	});
	const visibleLines = $derived(filteredLines.slice(0, windowLimit));
	const hasMore = $derived(filteredLines.length > windowLimit);

	const compareLines = $derived(splitLines(compareWith?.bytes));
	const filteredCompareLines = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return compareLines;
		return compareLines.filter((l) => l.toLowerCase().includes(q));
	});
	const visibleCompareLines = $derived(filteredCompareLines.slice(0, windowLimit));
	const hasMoreCompare = $derived(filteredCompareLines.length > windowLimit);

	const parserLabel = $derived.by(() => {
		switch (result.parserKind) {
			case Pb.ParserKind.TEXTFSM:
				return 'textfsm';
			case Pb.ParserKind.NATIVE_JSON:
				return 'native_json';
			case Pb.ParserKind.BUILTIN:
				return 'builtin';
			default:
				return '';
		}
	});

	function safeFileSegment(name: string): string {
		return name.replace(/[^a-zA-Z0-9._-]+/g, '_');
	}

	async function copyAll() {
		if (!result.bytes) return;
		try {
			await navigator.clipboard.writeText(decode(result.bytes));
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch (err) {
			console.error('Clipboard write failed:', err);
		}
	}

	function triggerDownload(blob: Blob, ext: string) {
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		const safeRouter = safeFileSegment(result.routerName);
		const safeCmd = safeFileSegment(result.resolvedCommand);
		a.href = url;
		a.download = `${safeRouter}-${safeCmd}-${new Date().toISOString().replace(/[:.]/g, '-')}.${ext}`;
		a.click();
		URL.revokeObjectURL(url);
	}

	function downloadTxt() {
		if (!result.bytes) return;
		triggerDownload(new Blob([result.bytes as unknown as BlobPart], { type: 'text/plain' }), 'txt');
	}

	function downloadJson() {
		if (!result.parsed) return;
		const json = JSON.stringify(
			result.parsed.data,
			(_k, v) => (typeof v === 'bigint' ? v.toString() : v),
			2
		);
		triggerDownload(new Blob([json], { type: 'application/json' }), 'json');
	}
</script>

<article
	class="lg-card flex flex-col overflow-hidden"
	style="max-height: calc(var(--sheet-body-height) - 2rem);"
	in:fade={{ duration: transitionDuration(150) }}
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
				{#if result.status === 'pending'}
					queued
				{:else if result.retryAttempt > 0}
					retrying (#{result.retryAttempt})…
				{:else}
					running…
				{/if}
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
					aria-label="Compare with previous run"
					value={compareEntryId}
					onchange={(e) => (compareEntryId = e.currentTarget.value)}
				>
					<option value="">Compare…</option>
					{#each pastRuns as past (past.entryId)}
						<option value={past.entryId}>vs {relativeTime(past.timestamp, $now)}</option>
					{/each}
				</select>
			{/if}

			{#if result.cached}
				<span
					class="lg-badge lg-badge-info tracking-wider uppercase"
					style="font-size: 9px; padding: 0.125rem 0.375rem; height: max-content;"
				>
					Cached
				</span>
			{/if}

			{#if result.timestamp}
				<time
					class="flex items-center gap-1 font-mono text-xs"
					style="color: var(--color-fg-subtle);"
					datetime={isoTime(result.timestamp)}
					title={absoluteTime(result.timestamp)}
				>
					<Clock size={11} class="opacity-60" />
					{relativeTime(result.timestamp, $now)}
				</time>
			{/if}
		</div>
	</header>

	{#if result.status === 'pending' || result.status === 'running'}
		<div class="flex justify-center p-8">
			<Loader
				label={result.retryAttempt > 0 ? `retrying (${result.retryAttempt})` : result.status}
				size="sm"
			/>
		</div>
	{:else if result.status === 'error'}
		<div
			class="flex items-start gap-2 p-4 text-sm"
			style="color: var(--color-danger); background-color: color-mix(in oklab, var(--color-danger) 8%, transparent);"
		>
			<CircleX size={16} class="mt-0.5 shrink-0" />
			<span class="font-mono break-words whitespace-pre-wrap">{result.error}</span>
		</div>
	{:else}
		<ResultToolbar
			comparing={compareWith !== null}
			{canStructured}
			compareHasParsed={compareWith?.parsed != null}
			{viewMode}
			{diffViewMode}
			{parserLabel}
			{search}
			filteredCount={filteredLines.length}
			totalCount={lines.length}
			{copied}
			onViewModeChange={(m) => (viewMode = m)}
			onDiffViewModeChange={(m) => (diffViewMode = m)}
			onSearchChange={(v) => (search = v)}
			onCopy={copyAll}
			onDownload={downloadTxt}
			onDownloadJson={downloadJson}
		/>

		{#if compareWith}
			{#if diffViewMode === 'diff'}
				<RawDiffView
					oldStr={decode(compareWith.bytes)}
					newStr={decode(result.bytes)}
					windowStep={WINDOW_STEP}
				/>
			{:else if diffViewMode === 'raw-split'}
				<RawOutputView
					split={true}
					{visibleLines}
					filteredCount={filteredLines.length}
					{visibleCompareLines}
					{hasMore}
					{hasMoreCompare}
					{windowLimit}
					windowStep={WINDOW_STEP}
					onShowMore={() => (windowLimit += WINDOW_STEP)}
				/>
			{:else if diffViewMode === 'structured-diff'}
				<StructuredView
					mode="diff"
					parsed={result.parsed}
					compareParsed={compareWith.parsed}
					routerId={result.routerId}
				/>
			{:else if diffViewMode === 'structured-split'}
				<StructuredView
					mode="split"
					parsed={result.parsed}
					compareParsed={compareWith.parsed}
					routerId={result.routerId}
					nextPageToken={result.nextPageToken}
					loadingMore={result.loadingMore}
					onLoadMore={() => void loadMorePeerRoutes(result.routerId)}
				/>
			{/if}
		{:else if canStructured && viewMode === 'structured' && result.parsed}
			<StructuredView
				mode="single"
				parsed={result.parsed}
				routerId={result.routerId}
				nextPageToken={result.nextPageToken}
				loadingMore={result.loadingMore}
				onLoadMore={() => void loadMorePeerRoutes(result.routerId)}
			/>
		{:else}
			<RawOutputView
				{visibleLines}
				filteredCount={filteredLines.length}
				{hasMore}
				{windowLimit}
				windowStep={WINDOW_STEP}
				onShowMore={() => (windowLimit += WINDOW_STEP)}
			/>
		{/if}
	{/if}
</article>
