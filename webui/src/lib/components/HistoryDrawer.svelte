<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import {
		history,
		deleteHistoryEntry,
		clearHistory,
		type HistoryEntry
	} from '$lib/stores/history';
	import { COMMANDS, restoreHistoryEntry, runWithRouters } from '$lib/stores/query';
	import { now, relativeTime, absoluteTime, isoTime } from '$lib/time';
	import { transitionDuration } from '$lib/motion';
	import X from '@lucide/svelte/icons/x';
	import History from '@lucide/svelte/icons/history';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import Eye from '@lucide/svelte/icons/eye';
	import Trash2 from '@lucide/svelte/icons/trash-2';

	interface Props {
		open: boolean;
		onClose: () => void;
	}
	let { open, onClose }: Props = $props();

	function labelFor(cmd: string): string {
		return COMMANDS.find((c) => c.value === cmd)?.label ?? cmd;
	}

	function viewEntry(entry: HistoryEntry) {
		restoreHistoryEntry(entry);
		onClose();
	}

	function rerunEntry(entry: HistoryEntry) {
		void runWithRouters(entry.command, entry.parameter, entry.routers);
		onClose();
	}

	function onKeyDown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			e.preventDefault();
			onClose();
		}
	}
</script>

<svelte:window onkeydown={onKeyDown} />

{#if open}
	<div
		class="fixed inset-0 z-50 flex justify-end"
		role="dialog"
		aria-modal="true"
		aria-label="Query history"
	>
		<button
			type="button"
			class="fixed inset-0 bg-black/40 backdrop-blur-[1px]"
			aria-label="Close history"
			onclick={onClose}
			transition:fade={{ duration: transitionDuration(150) }}
		></button>

		<aside
			class="relative z-10 flex h-full w-full max-w-md flex-col border-l shadow-2xl"
			style="background-color: var(--color-bg-mantle); border-color: var(--color-border);"
			transition:fly={{ x: 320, duration: transitionDuration(180) }}
		>
			<header
				class="flex h-14 shrink-0 items-center gap-2 border-b px-4"
				style="border-color: var(--color-border);"
			>
				<History size={16} class="opacity-70" />
				<h2 class="text-sm font-semibold tracking-wide uppercase">Query History</h2>
				{#if $history.length > 0}
					<span class="lg-badge">{$history.length}</span>
					<button
						type="button"
						class="lg-btn lg-btn-ghost ml-auto h-8 !px-2 text-xs"
						onclick={clearHistory}
						title="Clear all history"
					>
						<Trash2 size={13} />
						Clear all
					</button>
				{:else}
					<span class="ml-auto"></span>
				{/if}
				<button
					type="button"
					class="lg-btn lg-btn-ghost h-8 w-8 !p-0"
					aria-label="Close history panel"
					onclick={onClose}
				>
					<X size={16} />
				</button>
			</header>

			<div class="flex-1 overflow-y-auto p-3">
				{#if $history.length === 0}
					<p class="py-12 text-center text-sm" style="color: var(--color-fg-muted);">
						No executed queries recorded yet.
					</p>
				{:else}
					<ul class="flex flex-col gap-2">
						{#each $history as entry (entry.id)}
							<li class="lg-card flex flex-col gap-2 p-3">
								<div class="flex items-start justify-between gap-2">
									<div class="min-w-0 flex-1">
										<div class="flex flex-wrap items-center gap-1.5">
											<span class="lg-badge lg-badge-info">{labelFor(entry.command)}</span>
											<span class="text-xs" style="color: var(--color-fg-muted);">
												{entry.routers.length} router{entry.routers.length === 1 ? '' : 's'}
											</span>
										</div>
										{#if entry.parameter}
											<p class="mt-1 truncate font-mono text-xs" title={entry.parameter}>
												{entry.parameter}
											</p>
										{/if}
									</div>
									<time
										class="shrink-0 font-mono text-[11px]"
										style="color: var(--color-fg-subtle);"
										datetime={isoTime(entry.timestamp)}
										title={absoluteTime(entry.timestamp)}
									>
										{relativeTime(entry.timestamp, $now)}
									</time>
								</div>

								<div class="flex flex-wrap items-center gap-1.5">
									<button
										type="button"
										class="lg-btn lg-btn-outline h-7 !px-2 text-xs"
										onclick={() => viewEntry(entry)}
									>
										<Eye size={12} />
										View
									</button>
									<button
										type="button"
										class="lg-btn lg-btn-outline h-7 !px-2 text-xs"
										onclick={() => rerunEntry(entry)}
									>
										<RotateCcw size={12} />
										Re-run
									</button>
									<button
										type="button"
										class="lg-btn lg-btn-ghost ml-auto h-7 w-7 !p-0"
										aria-label="Delete history entry"
										title="Delete"
										onclick={() => deleteHistoryEntry(entry.id)}
									>
										<Trash2 size={12} />
									</button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</aside>
	</div>
{/if}
