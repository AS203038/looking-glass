<script lang="ts">
	import { onDestroy } from 'svelte';
	import DiffWorker from '$lib/workers/diff.worker.ts?worker';
	import type { DiffWorkerLine } from '$lib/workers/diff.worker';
	import Loader from '../Loader.svelte';

	interface Props {
		oldStr: string;
		newStr: string;
		windowStep?: number;
	}

	let { oldStr, newStr, windowStep = 500 }: Props = $props();

	let diffLinesArr = $state<DiffWorkerLine[]>([]);
	let diffLoading = $state(false);
	let diffError = $state<string | null>(null);
	let diffWindowLimit = $state(500);
	let diffWorker: Worker | null = null;
	let latestJobId = 0;

	$effect(() => {
		const prev = oldStr;
		const next = newStr;
		const jobId = ++latestJobId;

		diffLoading = true;
		diffError = null;
		diffLinesArr = [];
		diffWindowLimit = windowStep;

		if (!diffWorker) {
			diffWorker = new DiffWorker();
		}

		diffWorker.onmessage = (e) => {
			if (e.data.jobId !== latestJobId) return;
			if (e.data.type === 'success') {
				diffLinesArr = e.data.lines;
			} else {
				diffError = e.data.error;
			}
			diffLoading = false;
		};

		diffWorker.postMessage({ jobId, oldStr: prev, newStr: next });
	});

	onDestroy(() => {
		if (diffWorker) {
			diffWorker.terminate();
			diffWorker = null;
		}
	});

	const visibleDiffLines = $derived(diffLinesArr.slice(0, diffWindowLimit));
	const hasMoreDiff = $derived(diffLinesArr.length > diffWindowLimit);
</script>

{#if diffLoading}
	<div class="flex justify-center p-8"><Loader label="Computing diff…" size="sm" /></div>
{:else if diffError}
	<div class="p-4 text-sm" style="color: var(--color-danger);">
		Failed to compute diff: {diffError}
	</div>
{:else}
	<div
		class="flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed whitespace-pre"
		style="background-color: var(--color-bg-inset); color: var(--color-fg);"
	>
		{#each visibleDiffLines as part, i (i)}
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
				<span class="mr-1.5 inline-block w-3 opacity-75 select-none" aria-hidden="true">
					{part.added ? '+' : part.removed ? '-' : ' '}
				</span><span class="sr-only">
					{part.added ? 'Added: ' : part.removed ? 'Removed: ' : ''}
				</span>{part.value}
			</div>
		{/each}
	</div>
	{#if hasMoreDiff}
		<div
			class="flex items-center justify-between border-t px-4 py-2 text-xs"
			style="border-color: var(--color-border); color: var(--color-fg-muted); background-color: var(--color-bg);"
		>
			<span>Showing {diffWindowLimit.toLocaleString()} diff lines</span>
			<button
				type="button"
				class="lg-btn lg-btn-ghost h-7 !px-2 text-xs"
				onclick={() => (diffWindowLimit += windowStep)}
			>
				Show more
			</button>
		</div>
	{/if}
{/if}
