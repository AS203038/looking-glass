<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/env';
	import { selectedRouters, toggleSelection } from '$lib/stores/routers';
	import {
		COMMANDS,
		COMMANDS_NO_PARAM,
		command,
		parameter,
		isRunning,
		cancelRun,
		run,
		resolveCommand,
		parsePeerRoutesParam,
		formatPeerRoutesParam,
		type CommandValue,
		type PeerRouteQueryTypeName
	} from '$lib/stores/query';
	import { dockHeight } from '$lib/stores/sheet';
	import { history } from '$lib/stores/history';
	import { buildShareUrl } from '$lib/urlSync';
	import { pushToast } from '$lib/toast';
	import Play from '@lucide/svelte/icons/play';
	import Square from '@lucide/svelte/icons/square';
	import Terminal from '@lucide/svelte/icons/terminal';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import X from '@lucide/svelte/icons/x';
	import RouterIcon from '@lucide/svelte/icons/router';
	import Share2 from '@lucide/svelte/icons/share-2';

	let dockEl: HTMLElement | null = $state(null);

	const currentMeta = $derived(COMMANDS.find((c) => c.value === $command));
	const placeholder = $derived(currentMeta?.placeholder ?? 'Parameter…');
	const paramOptional = $derived(
		$command !== '' && (COMMANDS_NO_PARAM as readonly string[]).includes($command)
	);
	const isPeerRoutes = $derived($command === 'bgp_peer_routes');

	const peerParsed = $derived(parsePeerRoutesParam($parameter));

	const paramHistory = $derived.by(() => {
		if (!$command) return [];
		const params = new Set<string>();
		for (const h of $history) {
			if (h.command === $command && h.parameter) {
				params.add(h.parameter);
			}
		}
		return Array.from(params);
	});

	const validation = $derived.by(() => {
		if (!$command) return null;
		const trimmed = $parameter.trim();
		if ($command === 'bgp_peer_routes') {
			const { peerIp, queryType, peerName } = peerParsed;
			if (!peerIp && !peerName) {
				return null;
			}
			if (peerIp && /\s/.test(peerIp)) {
				return { kind: 'error' as const, message: 'Peer IP must not contain spaces' };
			}
			const who = peerIp || `name=${peerName}`;
			return {
				kind: 'info' as const,
				message: `Querying: ${queryType.toUpperCase()} routes for peer ${who}`
			};
		}

		if (!trimmed) return null;

		if ($command === 'bgp_community') {
			const parts = trimmed.split(':');
			if (parts.length !== 2 && parts.length !== 3) {
				return { kind: 'error' as const, message: 'Use ASN:VALUE or GLOBAL:LOCAL1:LOCAL2' };
			}
			if (parts.some((p) => !/^\d+$/.test(p))) {
				return { kind: 'error' as const, message: 'All community segments must be numeric' };
			}
			const nums = parts.map((p) => Number(p));
			const max = parts.length === 2 ? 65535 : 4294967295;
			if (nums.some((n) => !Number.isSafeInteger(n) || n < 0 || n > max)) {
				return {
					kind: 'error' as const,
					message:
						parts.length === 2
							? 'Standard community segments must be between 0 and 65535'
							: 'Large community segments must be between 0 and 4294967295'
				};
			}
			return {
				kind: 'info' as const,
				message:
					parts.length === 3
						? 'Detected: BGP Large Community (RFC 8092)'
						: 'Detected: BGP Standard Community (RFC 1997)'
			};
		}

		if ($command === 'ping' || $command === 'traceroute' || $command === 'bgp_route') {
			if (/\s/.test(trimmed) || !/^[a-zA-Z0-9.:/_-]+$/.test(trimmed)) {
				return {
					kind: 'error' as const,
					message: 'Enter a single IP address, CIDR prefix, or hostname without spaces'
				};
			}
		}

		return null;
	});

	const hasRequiredParam = $derived.by(() => {
		if (paramOptional) return true;
		if (isPeerRoutes) return !!(peerParsed.peerIp || peerParsed.peerName);
		return $parameter.trim() !== '';
	});

	const canSubmit = $derived(
		!$isRunning &&
			$selectedRouters.length > 0 &&
			$command !== '' &&
			hasRequiredParam &&
			validation?.kind !== 'error'
	);

	function onCmdChange(e: Event) {
		const v = (e.currentTarget as HTMLSelectElement).value as CommandValue | '';
		command.set(v);
	}

	function onParamInput(e: Event) {
		parameter.set((e.currentTarget as HTMLInputElement).value);
	}

	function updatePeerRoutes(
		patch: Partial<{ peerIp: string; queryType: PeerRouteQueryTypeName; peerName: string }>
	) {
		const next = { ...peerParsed, ...patch };
		parameter.set(formatPeerRoutesParam(next.peerIp, next.queryType, next.peerName));
	}

	function scrollToRouter(id: bigint) {
		const el = document.querySelector(`[data-router-id="${id}"]`);
		if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
	}

	async function submit(e?: Event) {
		e?.preventDefault();
		if (!canSubmit || !$command) return;
		await run($command, paramOptional ? '' : $parameter.trim());
	}

	async function copyShareLink() {
		if (!browser) return;
		const url = buildShareUrl(window.location.href, {
			routerNames: $selectedRouters.map((r) => r.name),
			cmd: $command,
			param: $parameter
		});
		try {
			await navigator.clipboard.writeText(url);
			pushToast({ kind: 'success', title: 'Shareable link copied', ttl: 2500 });
		} catch (err) {
			console.error('Clipboard write failed:', err);
			pushToast({ kind: 'error', title: 'Could not copy link' });
		}
	}

	function onWindowKeyDown(e: KeyboardEvent) {
		if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && canSubmit) {
			e.preventDefault();
			void submit();
		}
	}

	onMount(() => {
		if (!dockEl || typeof ResizeObserver === 'undefined') return;
		const ro = new ResizeObserver((entries) => {
			for (const entry of entries) {
				const h = Math.ceil(entry.contentRect.height);
				dockHeight.set(h);
			}
		});
		ro.observe(dockEl);
		return () => ro.disconnect();
	});
</script>

<svelte:window onkeydown={onWindowKeyDown} />

<aside
	bind:this={dockEl}
	class="border-t backdrop-blur-md"
	style="background-color: color-mix(in oklab, var(--color-bg-mantle) 92%, transparent);
	       border-color: var(--color-border);"
	aria-label="Command dock"
>
	<div class="mx-auto flex max-w-7xl flex-col gap-2 px-4 py-2 sm:px-6 sm:py-3">
		{#if $selectedRouters.length === 0}
			<p class="text-xs" style="color: var(--color-fg-subtle);">
				Pick at least one router above to enable execution.
			</p>
		{:else}
			<div class="flex items-center gap-1.5">
				<span
					class="lg-badge lg-badge-info shrink-0"
					title="{$selectedRouters.length} router{$selectedRouters.length === 1
						? ''
						: 's'} selected"
				>
					<RouterIcon size={12} />
					{$selectedRouters.length}
				</span>
				<div
					class="-mx-1 flex flex-1 items-center gap-1.5 overflow-x-auto px-1"
					role="list"
					aria-label="Selected routers"
					style="mask-image: linear-gradient(to right, black calc(100% - 24px), transparent);
					       -webkit-mask-image: linear-gradient(to right, black calc(100% - 24px), transparent);
					       scrollbar-width: none;"
				>
					{#each $selectedRouters as r (r.id)}
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
								class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-r-full hover:bg-(--color-surface-hover)"
								onclick={() => toggleSelection(r.id)}
								aria-label={`Deselect ${r.name}`}
								title={`Deselect ${r.name}`}
							>
								<X size={10} />
							</button>
						</span>
					{/each}
				</div>
			</div>
		{/if}

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
				<select id="lg-cmd" class="lg-input py-2" value={$command} onchange={onCmdChange}>
					<option value="" disabled>Select an action…</option>
					{#each COMMANDS as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			{#if isPeerRoutes}
				<div class="flex flex-1 flex-wrap items-end gap-2">
					<div class="flex min-w-[10rem] flex-1 flex-col gap-1">
						<label
							for="lg-peer-ip"
							class="text-[0.65rem] font-medium tracking-wide uppercase"
							style="color: var(--color-fg-muted);"
						>
							Peer IP
						</label>
						<input
							id="lg-peer-ip"
							type="text"
							class="lg-input py-2 font-mono"
							placeholder="192.0.2.1 (or use Peer Name)"
							value={peerParsed.peerIp}
							oninput={(e) => updatePeerRoutes({ peerIp: e.currentTarget.value })}
							autocomplete="off"
							autocapitalize="off"
							autocorrect="off"
							spellcheck="false"
						/>
					</div>
					<div class="flex min-w-[9rem] flex-1 flex-col gap-1">
						<label
							for="lg-peer-name"
							class="text-[0.65rem] font-medium tracking-wide uppercase"
							style="color: var(--color-fg-muted);"
						>
							Peer Name
						</label>
						<input
							id="lg-peer-name"
							type="text"
							class="lg-input py-2 font-mono"
							placeholder="optional peer name"
							value={peerParsed.peerName}
							oninput={(e) => updatePeerRoutes({ peerName: e.currentTarget.value })}
							autocomplete="off"
							autocapitalize="off"
							autocorrect="off"
							spellcheck="false"
						/>
					</div>
					<div class="flex w-32 flex-col gap-1">
						<label
							for="lg-peer-type"
							class="text-[0.65rem] font-medium tracking-wide uppercase"
							style="color: var(--color-fg-muted);"
						>
							RIB
						</label>
						<select
							id="lg-peer-type"
							class="lg-input py-2"
							value={peerParsed.queryType}
							onchange={(e) =>
								updatePeerRoutes({
									queryType: e.currentTarget.value as PeerRouteQueryTypeName
								})}
						>
							<option value="received">received</option>
							<option value="accepted">accepted</option>
							<option value="rejected">rejected</option>
							<option value="advertised">advertised</option>
						</select>
					</div>
					<div class="flex items-center gap-1.5">
						<button
							type="button"
							class="lg-btn lg-btn-outline h-10 w-10 shrink-0 !p-0"
							onclick={copyShareLink}
							aria-label="Copy shareable link"
							title="Copy shareable link"
						>
							<Share2 size={15} />
						</button>
						{#if $isRunning}
							<button
								type="button"
								class="lg-btn lg-btn-outline h-10 shrink-0 px-4"
								onclick={cancelRun}
							>
								<Square size={14} />
								<span>Cancel</span>
							</button>
						{:else}
							<button
								type="submit"
								class="lg-btn lg-btn-primary h-10 shrink-0 px-4 sm:px-5"
								disabled={!canSubmit}
							>
								<Play size={16} />
								<span>Execute</span>
							</button>
						{/if}
					</div>
				</div>
			{:else}
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
								class="lg-input py-2 pl-9 font-mono disabled:cursor-not-allowed disabled:opacity-50"
								{placeholder}
								value={paramOptional ? '' : $parameter}
								disabled={paramOptional}
								oninput={onParamInput}
								autocomplete="off"
								autocapitalize="off"
								autocorrect="off"
								spellcheck="false"
								inputmode="text"
								list="lg-param-history"
							/>
							<datalist id="lg-param-history">
								{#each paramHistory as p (p)}
									<option value={p}></option>
								{/each}
							</datalist>
						</div>
					</div>

					<button
						type="button"
						class="lg-btn lg-btn-outline h-10 w-10 shrink-0 !p-0"
						onclick={copyShareLink}
						aria-label="Copy shareable link"
						title="Copy shareable link"
					>
						<Share2 size={15} />
					</button>

					{#if $isRunning}
						<button
							type="button"
							class="lg-btn lg-btn-outline h-10 shrink-0 px-4"
							onclick={cancelRun}
						>
							<Square size={14} />
							<span>Cancel</span>
						</button>
					{:else}
						<button
							type="submit"
							class="lg-btn lg-btn-primary h-10 shrink-0 px-4 sm:px-5"
							disabled={!canSubmit}
						>
							<Play size={16} />
							<span>Execute</span>
						</button>
					{/if}
				</div>
			{/if}
		</form>

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
		{:else if $selectedRouters.length > 0 && $command && (paramOptional || $parameter.trim())}
			<p class="hidden text-[0.7rem] sm:block" style="color: var(--color-fg-subtle);">
				Will run <span class="font-mono">
					{resolveCommand($command as CommandValue, $parameter.trim())}
				</span>
				{#if !paramOptional}
					<span class="font-mono">"{$parameter.trim()}"</span>
				{/if}
				on {$selectedRouters.length} router{$selectedRouters.length === 1 ? '' : 's'}.
			</p>
		{/if}
	</div>
</aside>
