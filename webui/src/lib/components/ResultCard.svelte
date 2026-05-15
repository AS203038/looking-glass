<script lang="ts">
	/**
	 * Single-router result card.
	 *
	 * Rendering policy:
	 *   1. If the server returned a structured payload (`result.parsed`
	 *      set with parse_status == OK), default to the matching
	 *      `*View.svelte` component.
	 *   2. A "Raw" toggle in the toolbar swaps into the verbatim
	 *      output viewer (windowed line list, inline filter, copy,
	 *      download). This path is the only path when no parser ran
	 *      or when parsing failed.
	 *   3. A parser-provenance chip in the toolbar lets operators see
	 *      which pipe produced the structured view ("textfsm",
	 *      "native_json", "builtin"), useful when debugging vendor
	 *      output drift.
	 *
	 * Why a per-card toggle and not a global preference:
	 *   Different ops want different defaults. Ping wants the stat
	 *   strip 100% of the time. `bgp.route` for an unfamiliar prefix
	 *   sometimes wants raw to copy/paste full attribute strings into
	 *   a ticket. Per-card persistence would be overkill; a quick
	 *   toggle is enough.
	 */
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

	interface Props {
		result: ExecResult;
	}
	let { result }: Props = $props();

	let copied = $state(false);
	let search = $state('');
	let windowLimit = $state(500);
	const WINDOW_STEP = 500;
	// Live-ticking reference for relative-time rendering.
	let nowDate = $state(new Date());
	now.subscribe((d) => (nowDate = d));

	// "structured" when the server gave us a parsed payload; the user
	// can flip to "raw" to inspect the verbatim bytes.
	let viewMode = $state<'structured' | 'raw'>('structured');
	const canStructured = $derived(result.parsed != null);
	// Reset to structured whenever the result mutates (e.g. a fresh
	// run replaces the previous one).
	$effect(() => {
		if (result.parsed != null && viewMode === 'raw') {
			// keep the user's manual choice within the same result
		}
	});

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

	// Map ParserKind enum (numeric in TS) to a label. Avoid importing
	// the enum-as-value to keep this file's TS surface light.
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

<article class="lg-card flex flex-col overflow-hidden" in:fade={{ duration: 150 }}>
	<!-- Header -->
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

		<!-- Status -->
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
	</header>

	<!-- Body -->
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
		<!-- Toolbar: view toggle + parser provenance + filter (raw only) + copy/download -->
		<div
			class="flex flex-wrap items-center gap-2 border-b px-3 py-2"
			style="border-color: var(--color-border);"
		>
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
		</div>

		<!-- Output -->
		{#if canStructured && viewMode === 'structured' && result.parsed}
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
			<pre
				class="max-h-96 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
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
