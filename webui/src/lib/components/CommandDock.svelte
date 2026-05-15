<script lang="ts">
	/**
	 * CommandDock — the persistent action surface, anchored at the bottom
	 * of the viewport via the shared sticky wrapper in `+layout.svelte`.
	 *
	 * The dock used to be `position: sticky; bottom: 0` on its own, sharing
	 * that anchor with the ResultsSheet — which caused the sheet to overlap
	 * the dock on mobile once the sheet grew tall. The shared sticky
	 * wrapper now guarantees they stack (sheet above, dock below) and the
	 * sheet's max-height is computed against this dock's measured height
	 * (`dockHeight` store, published below via ResizeObserver). The wrapper
	 * also owns `padding-bottom: env(safe-area-inset-bottom)` so this
	 * component no longer has to.
	 *
	 * Mobile-first form layout:
	 *   - Below `sm` (640 px) the form is two rows: Action on top, then
	 *     [Parameter | Execute] on a single row. This collapses the dock
	 *     from ~220 px to ~140 px on portrait phones, giving the sheet
	 *     dramatically more room.
	 *   - The "Will run X on Y routers." preview is hidden on `< sm` —
	 *     it's nice on desktop but eats a precious line on mobile and is
	 *     redundant with the action/parameter inputs being right there.
	 *   - Selection chip strip gets a right-edge fade mask so users can
	 *     see at a glance that it's horizontally scrollable.
	 */
	import { onMount } from 'svelte';
	import { selectedRouters, toggleSelection } from '$lib/stores/routers';
	import {
		COMMANDS,
		COMMANDS_NO_PARAM,
		command,
		parameter,
		run,
		resolveCommand,
		type CommandValue
	} from '$lib/stores/query';
	import { dockHeight } from '$lib/stores/sheet';
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
	let dockEl: HTMLElement | null = $state(null);

	command.subscribe((v) => (cmd = v));
	parameter.subscribe((v) => (param = v));
	selectedRouters.subscribe((v) => (selected = v));

	const currentMeta = $derived(COMMANDS.find((c) => c.value === cmd));
	const placeholder = $derived(currentMeta?.placeholder ?? 'Parameter…');
	// True when the selected command does not require a parameter
	// (currently only `bgp_summary`). Used to relax the param-empty
	// guards in canSubmit / disabled state of the parameter input.
	const paramOptional = $derived(
		cmd !== '' && (COMMANDS_NO_PARAM as readonly string[]).includes(cmd)
	);

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
			(paramOptional || param.trim() !== '') &&
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

	// Measure the dock and publish to the shared `dockHeight` store so
	// the ResultsSheet can subtract it from its own max-height. We use
	// ResizeObserver so this stays accurate when:
	//   - the chip strip wraps to a second visual line
	//   - the validation hint appears/disappears
	//   - the soft keyboard opens (iOS shifts `visualViewport` not the
	//     element box, but Safari still fires a resize on the dock when
	//     the layout reflows around the keyboard)
	//   - the user rotates the device (chromium fires it via the
	//     surrounding flex container)
	onMount(() => {
		if (!dockEl || typeof ResizeObserver === 'undefined') return;
		const ro = new ResizeObserver((entries) => {
			for (const entry of entries) {
				// `contentRect.height` excludes the wrapper's safe-area
				// padding (which lives on the parent) — exactly the value
				// the sheet should reserve. Round up to avoid jitter when
				// sub-pixel layout produces values like 119.6 → 119.7 → …
				const h = Math.ceil(entry.contentRect.height);
				dockHeight.set(h);
			}
		});
		ro.observe(dockEl);
		return () => ro.disconnect();
	});
</script>

<aside
	bind:this={dockEl}
	class="border-t backdrop-blur-md"
	style="background-color: color-mix(in oklab, var(--color-bg-mantle) 92%, transparent);
	       border-color: var(--color-border);"
	aria-label="Command dock"
>
	<div class="mx-auto flex max-w-7xl flex-col gap-2 px-4 py-2 sm:px-6 sm:py-3">
		<!-- Selection chip strip -->
		{#if selected.length === 0}
			<p class="text-xs" style="color: var(--color-fg-subtle);">
				Pick at least one router above to enable execution.
			</p>
		{:else}
			<!-- Right-edge fade mask hints that the strip scrolls horizontally
				 when there are more chips than fit on screen. -->
			<div
				class="-mx-1 flex items-center gap-1.5 overflow-x-auto px-1"
				role="list"
				aria-label="Selected routers"
				style="mask-image: linear-gradient(to right, black calc(100% - 24px), transparent);
				       -webkit-mask-image: linear-gradient(to right, black calc(100% - 24px), transparent);
				       scrollbar-width: none;"
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
							class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-r-full hover:bg-(--color-surface-hover) sm:h-5 sm:w-5"
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

		<!--
			Form layout:
			  mobile (<sm): two rows — Action on its own, then a
			                [Parameter | Execute] row.
			  ≥sm:          single row of [Action | Parameter | Execute],
			                items aligned to the row's end so they line
			                up regardless of label height.

			Implementation: outer `flex-col`. Inner `param-and-execute` row
			is `flex` on both, but only contains both items together; on
			≥sm the outer turns to `flex-row` and the action input becomes
			a peer to that row.
		-->
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

			<!-- Parameter + Execute share a row on mobile (and continue to
				 look natural on desktop because the outer flex switches to
				 row at sm:). The Execute button gets a fixed-min-width on
				 mobile to keep the input from collapsing to nothing. -->
			<div class="flex flex-1 items-end gap-2">
				<div class="flex min-w-0 flex-1 flex-col gap-1">
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
							autocapitalize="off"
							autocorrect="off"
							spellcheck="false"
							inputmode="text"
						/>
					</div>
				</div>

				<button
					type="submit"
					class="lg-btn lg-btn-primary h-10 shrink-0 px-4 sm:px-5"
					disabled={!canSubmit}
					aria-disabled={!canSubmit}
				>
					<Play size={16} />
					<span>{submitting ? 'Executing…' : 'Execute'}</span>
				</button>
			</div>
		</form>

		<!-- Validation (errors always visible; the verbose preview is
			 desktop-only — it's redundant on mobile where every input is
			 inches from the user's thumb). -->
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
			<p class="hidden text-[0.7rem] sm:block" style="color: var(--color-fg-subtle);">
				Will run <span class="font-mono">{resolveCommand(cmd as CommandValue, param)}</span>
				<span class="font-mono">"{param}"</span> on {selected.length} router{selected.length === 1
					? ''
					: 's'}.
			</p>
		{/if}
	</div>
</aside>
