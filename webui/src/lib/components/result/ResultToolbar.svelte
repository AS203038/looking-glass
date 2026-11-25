<script lang="ts">
	import Copy from '@lucide/svelte/icons/copy';
	import Download from '@lucide/svelte/icons/download';
	import FileJson from '@lucide/svelte/icons/file-json';
	import Search from '@lucide/svelte/icons/search';
	import Table2 from '@lucide/svelte/icons/table-2';
	import FileText from '@lucide/svelte/icons/file-text';
	import FileDiff from '@lucide/svelte/icons/file-diff';

	export type ViewMode = 'structured' | 'raw';
	export type DiffViewMode = 'diff' | 'raw-split' | 'structured-split' | 'structured-diff';

	interface Props {
		comparing: boolean;
		canStructured: boolean;
		compareHasParsed: boolean;
		viewMode: ViewMode;
		diffViewMode: DiffViewMode;
		parserLabel: string;
		search: string;
		filteredCount: number;
		totalCount: number;
		copied: boolean;
		onViewModeChange: (mode: ViewMode) => void;
		onDiffViewModeChange: (mode: DiffViewMode) => void;
		onSearchChange: (value: string) => void;
		onCopy: () => void;
		onDownload: () => void;
		onDownloadJson?: () => void;
	}

	let {
		comparing,
		canStructured,
		compareHasParsed,
		viewMode,
		diffViewMode,
		parserLabel,
		search,
		filteredCount,
		totalCount,
		copied,
		onViewModeChange,
		onDiffViewModeChange,
		onSearchChange,
		onCopy,
		onDownload,
		onDownloadJson
	}: Props = $props();
</script>

<div
	class="flex flex-wrap items-center gap-2 border-b px-3 py-2"
	style="border-color: var(--color-border);"
>
	{#if comparing}
		<div
			class="inline-flex overflow-hidden rounded-md border"
			style="border-color: var(--color-border);"
		>
			<button
				type="button"
				class="flex items-center gap-1 px-2 py-1 text-xs"
				class:font-semibold={diffViewMode === 'diff'}
				style:background-color={diffViewMode === 'diff' ? 'var(--color-bg-inset)' : 'transparent'}
				onclick={() => onDiffViewModeChange('diff')}
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
				onclick={() => onDiffViewModeChange('raw-split')}
				title="Raw Side-by-Side"
			>
				<FileText size={12} />
				Raw Split
			</button>
			{#if canStructured || compareHasParsed}
				<button
					type="button"
					class="flex items-center gap-1 px-2 py-1 text-xs"
					class:font-semibold={diffViewMode === 'structured-diff'}
					style:background-color={diffViewMode === 'structured-diff'
						? 'var(--color-bg-inset)'
						: 'transparent'}
					onclick={() => onDiffViewModeChange('structured-diff')}
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
					onclick={() => onDiffViewModeChange('structured-split')}
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
					onclick={() => onViewModeChange('structured')}
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
					onclick={() => onViewModeChange('raw')}
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
					title="Parser provenance"
				>
					via {parserLabel}
				</span>
			{/if}
		{:else if parserLabel}
			<span
				class="font-mono text-[10px]"
				style="color: var(--color-fg-subtle);"
				title="Parser attempted but produced no structured view"
			>
				parser: {parserLabel} (failed)
			</span>
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
					aria-label="Filter raw output lines"
					value={search}
					oninput={(e) => onSearchChange(e.currentTarget.value)}
				/>
			</div>
			<span class="ml-auto font-mono text-xs" style="color: var(--color-fg-subtle);">
				{filteredCount} / {totalCount} lines
			</span>
		{:else}
			<span class="ml-auto"></span>
		{/if}

		<button
			type="button"
			class="lg-btn lg-btn-outline h-8 !px-2.5 text-xs"
			onclick={onCopy}
			title="Copy full output to clipboard"
		>
			<Copy size={12} />
			{copied ? 'Copied' : 'Copy'}
		</button>
		<button
			type="button"
			class="lg-btn lg-btn-outline h-8 !px-2.5 text-xs"
			onclick={onDownload}
			title="Download full output as .txt"
		>
			<Download size={12} />
			Download
		</button>
		{#if canStructured && onDownloadJson}
			<button
				type="button"
				class="lg-btn lg-btn-outline h-8 !px-2.5 text-xs"
				onclick={onDownloadJson}
				title="Export structured payload as .json"
			>
				<FileJson size={12} />
				JSON
			</button>
		{/if}
	{/if}
</div>
