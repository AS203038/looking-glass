<script lang="ts">
	/**
	 * CommandDock — the persistent, sticky-bottom action surface.
	 *
	 * Replaces the in-flow `<CommandBar>` we shipped initially. The original
	 * UX put the command form *under* the router list; for operators with
	 * many locations the form could be 3+ pages of scrolling away, defeating
	 * the rapid iterate-and-execute workflow.
	 *
	 * Design notes:
	 *   - `position: sticky; bottom: 0` keeps it reachable at all viewport
	 *     sizes without leaving the document flow (so we can size `<main>`'s
	 *     bottom padding from the dock's own height — no fragile JS math).
	 *   - Selection is surfaced *here*, not in the picker. The dock is the
	 *     "what am I about to do, with what" surface; the picker becomes
	 *     pure discovery. Chips have an `×` to deselect without scrolling
	 *     back to the picker, and tapping a chip scrolls that router into
	 *     view (delegated via the `lg-router-anchor` data attribute).
	 *   - Validation lives directly under the parameter input; we keep the
	 *     same community-arity detection we already had in CommandBar.
	 *   - On submit, results materialise in the `<ResultsSheet>` directly
	 *     above the dock — no scroll needed. The sheet handles its own
	 *     hidden → peek transition; we just kick off the query.
	 *   - iOS safe-area-inset-bottom is honoured so the home indicator
	 *     doesn't overlap.
	 */
	import { selectedRouters, toggleSelection } from '$lib/stores/routers';
	import {
		COMMANDS,
		command,
		parameter,
		run,
		resolveCommand,
		type CommandValue
	} from '$lib/stores/query';
	import type { Pb } from '$lib/grpc';
	import Play from '@lucide/svelte/icons/play';
	import Terminal from '@lucide/svelte/icons/terminal';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import X from '@lucide/svelte/icons/x';
	import RouterIcon from '@lucide/svelte/icons/router';

	let cmd = $state<CommandValue | ''>('');
	let param = $state('');
	let selected: Pb.Router[] = $state([]);
	let submitting = $state(false);

	command.subscribe((v) => (cmd = v));
	parameter.subscribe((v) => (param = v));
	selectedRouters.subscribe((v) => (selected = v));

	const currentMeta = $derived(COMMANDS.find((c) => c.value === cmd));
	const placeholder = $derived(currentMeta?.placeholder ?? 'Parameter…');

	const validation = $derived.by(() => {
		if (!cmd || !param) return null;
		if (cmd === 'bgp_community') {
			const parts = param.split(':');
			if (parts.length !== 2 && parts.length !== 3) {
				return { kind: 'error' as const, message: 'Use ASN:VALUE or GLOBAL:LOCAL1:LOCAL2' };
			}
			if (parts.some((p) => !/^\d+$/.test(p))) {
				return { kind: 'error' as const, message: 'All community segments must be numeric' };
			}
			return {
				kind: 'info' as const,
				message:
					parts.length === 3
						? 'Detected: BGP Large Community (RFC 8092)'
						: 'Detected: BGP Standard Community (RFC 1997)'
			};
		}
		return null;
	});

	const canSubmit = $derived(
		!submitting &&
			selected.length > 0 &&
			cmd !== '' &&
			param.trim() !== '' &&
			validation?.kind !== 'error'
	);

	function onCmdChange(e: Event) {
		const v = (e.currentTarget as HTMLSelectElement).value as CommandValue | '';
		cmd = v;
		command.set(v);
	}

	function onParamInput(e: Event) {
		const v = (e.currentTarget as HTMLInputElement).value;
		param = v;
		parameter.set(v);
	}

	function scrollToRouter(id: bigint) {
		const el = document.querySelector(`[data-router-id="${id}"]`);
		if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
	}

	async function submit(e?: Event) {
		e?.preventDefault();
		if (!canSubmit || !cmd) return;
		submitting = true;
		try {
			await run(cmd, param.trim());
		} finally {
			submitting = false;
		}
	}
</script>

<aside
	class="sticky bottom-0 z-20 mt-auto border-t backdrop-blur-md"
	style="background-color: color-mix(in oklab, var(--color-bg-mantle) 92%, transparent);
	       border-color: var(--color-border);
	       padding-bottom: env(safe-area-inset-bottom);"
	aria-label="Command dock"
>
	<div class="mx-auto flex max-w-7xl flex-col gap-2 px-4 py-3 sm:px-6">
		<!-- Selection chip strip -->
		{#if selected.length === 0}
			<p class="text-xs" style="color: var(--color-fg-subtle);">
				Pick at least one router above to enable execution.
			</p>
		{:else}
			<div
				class="-mx-1 flex items-center gap-1.5 overflow-x-auto px-1"
				role="list"
				aria-label="Selected routers"
			>
				<span
					class="lg-badge lg-badge-info shrink-0"
					title="{selected.length} router{selected.length === 1 ? '' : 's'} selected"
				>
					<RouterIcon size={12} />
					{selected.length}
				</span>
				{#each selected as r (r.id)}
					<!-- A chip is composed of two interactive zones (scroll-to vs.
						 deselect-×), so it can't be a single nested button. We
						 render the chip as a flex row of two real <button>s
						 that visually fuse via a shared rounded background. -->
					<span
						role="listitem"
						class="lg-badge group flex shrink-0 items-center !p-0 hover:bg-(--color-surface)"
					>
						<button
							type="button"
							class="flex max-w-[10rem] items-center px-2 py-0.5 text-xs"
							onclick={() => scrollToRouter(r.id)}
							title={`Scroll to ${r.name}${r.location ? ` (${r.location})` : ''}`}
						>
							<span class="truncate">{r.name}</span>
						</button>
						<button
							type="button"
							class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-r-full hover:bg-(--color-surface-hover)"
							onclick={() => toggleSelection(r.id)}
							aria-label={`Deselect ${r.name}`}
							title={`Deselect ${r.name}`}
						>
							<X size={10} />
						</button>
					</span>
				{/each}
			</div>
		{/if}

		<!-- Form row -->
		<form
			class="flex flex-col gap-2 sm:flex-row sm:items-end"
			onsubmit={submit}
			aria-label="Looking glass query"
		>
			<div class="flex flex-col gap-1 sm:w-48">
				<label
					for="lg-cmd"
					class="text-[0.65rem] font-medium tracking-wide uppercase"
					style="color: var(--color-fg-muted);"
				>
					Action
				</label>
				<select id="lg-cmd" class="lg-input py-2" value={cmd} onchange={onCmdChange}>
					<option value="" disabled>Select an action…</option>
					{#each COMMANDS as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<div class="flex flex-1 flex-col gap-1">
				<label
					for="lg-param"
					class="text-[0.65rem] font-medium tracking-wide uppercase"
					style="color: var(--color-fg-muted);"
				>
					Parameter
				</label>
				<div class="relative">
					<Terminal
						size={14}
						class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 opacity-60"
					/>
					<input
						id="lg-param"
						type="text"
						class="lg-input py-2 pl-9 font-mono"
						{placeholder}
						value={param}
						oninput={onParamInput}
						autocomplete="off"
						spellcheck="false"
					/>
				</div>
			</div>

			<button
				type="submit"
				class="lg-btn lg-btn-primary h-10 px-5 sm:self-end"
				disabled={!canSubmit}
				aria-disabled={!canSubmit}
			>
				<Play size={16} />
				<span>{submitting ? 'Executing…' : 'Execute'}</span>
			</button>
		</form>

		<!-- Validation -->
		{#if validation}
			<p
				class="flex items-center gap-1.5 text-xs"
				style="color: {validation.kind === 'error'
					? 'var(--color-danger)'
					: 'var(--color-fg-muted)'};"
				aria-live="polite"
			>
				{#if validation.kind === 'error'}
					<CircleAlert size={12} />
				{/if}
				{validation.message}
			</p>
		{:else if selected.length > 0 && cmd && param}
			<p class="text-[0.7rem]" style="color: var(--color-fg-subtle);">
				Will run <span class="font-mono">{resolveCommand(cmd as CommandValue, param)}</span>
				<span class="font-mono">"{param}"</span> on {selected.length} router{selected.length === 1
					? ''
					: 's'}.
			</p>
		{/if}
	</div>
</aside>
