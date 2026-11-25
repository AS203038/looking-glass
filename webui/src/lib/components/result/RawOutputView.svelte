<script lang="ts">
	interface Props {
		split?: boolean;
		visibleLines: string[];
		filteredCount: number;
		visibleCompareLines?: string[];
		hasMore: boolean;
		hasMoreCompare?: boolean;
		windowLimit: number;
		windowStep: number;
		onShowMore: () => void;
	}

	let {
		split = false,
		visibleLines,
		filteredCount,
		visibleCompareLines = [],
		hasMore,
		hasMoreCompare = false,
		windowLimit,
		windowStep,
		onShowMore
	}: Props = $props();
</script>

{#if split}
	<div
		class="grid flex-1 grid-cols-1 divide-y overflow-hidden md:grid-cols-2 md:divide-x md:divide-y-0"
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
				style="color: var(--color-fg);"><code>{visibleCompareLines.join('\n')}</code></pre>
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
				style="color: var(--color-fg);"><code>{visibleLines.join('\n')}</code></pre>
		</div>
	</div>
	{#if hasMore || hasMoreCompare}
		<div
			class="flex items-center justify-between border-t px-4 py-2 text-xs"
			style="border-color: var(--color-border); color: var(--color-fg-muted); background-color: var(--color-bg);"
		>
			<span>Showing {windowLimit.toLocaleString()} lines</span>
			<button type="button" class="lg-btn lg-btn-ghost h-7 !px-2 text-xs" onclick={onShowMore}>
				Show more
			</button>
		</div>
	{/if}
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
				Showing {windowLimit.toLocaleString()} of {filteredCount.toLocaleString()} lines
			</span>
			<button type="button" class="lg-btn lg-btn-ghost h-7 !px-2 text-xs" onclick={onShowMore}>
				Show {Math.min(windowStep, filteredCount - windowLimit)} more
			</button>
		</div>
	{/if}
{/if}
