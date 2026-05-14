<script lang="ts">
	import { getEnv, parseLinks } from '$lib/env';
	import Menu from '@lucide/svelte/icons/menu';
	import X from '@lucide/svelte/icons/x';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import ThemeToggle from './ThemeToggle.svelte';

	const env = getEnv();
	const links = parseLinks(env.PUBLIC_HEADER_LINKS);

	let drawerOpen = $state(false);
	function closeDrawer() {
		drawerOpen = false;
	}
</script>

<header
	class="sticky top-0 z-30 border-b backdrop-blur-md"
	style="background-color: color-mix(in oklab, var(--color-bg-mantle) 85%, transparent); border-color: var(--color-border);"
>
	<div class="mx-auto flex h-14 max-w-7xl items-center gap-3 px-4 sm:px-6">
		<a
			href="/"
			class="flex min-w-0 items-center gap-3 text-base font-semibold tracking-wide uppercase"
		>
			{#if env.PUBLIC_HEADER_LOGO}
				<img src={env.PUBLIC_HEADER_LOGO} alt="Logo" class="h-8 w-auto shrink-0" />
			{/if}
			<span class="truncate">{env.PUBLIC_HEADER_TEXT || 'Looking Glass'}</span>
		</a>

		<nav class="ml-auto hidden items-center gap-1 md:flex">
			{#each links as { name, href } (href)}
				<a {href} target="_blank" rel="noreferrer" class="lg-btn lg-btn-ghost h-9 !px-3 text-sm">
					<span>{name}</span>
					<ExternalLink size={14} class="opacity-60" />
				</a>
			{/each}
			<div class="ml-1"><ThemeToggle /></div>
		</nav>

		<div class="ml-auto flex items-center gap-1 md:hidden">
			<ThemeToggle />
			{#if links.length > 0}
				<button
					type="button"
					class="lg-btn lg-btn-ghost h-9 w-9 !p-0"
					aria-label="Open navigation menu"
					aria-expanded={drawerOpen}
					onclick={() => (drawerOpen = !drawerOpen)}
				>
					{#if drawerOpen}<X size={18} />{:else}<Menu size={18} />{/if}
				</button>
			{/if}
		</div>
	</div>

	{#if drawerOpen}
		<div
			class="border-t md:hidden"
			style="background-color: var(--color-bg-mantle); border-color: var(--color-border);"
		>
			<nav class="mx-auto flex max-w-7xl flex-col gap-1 px-4 py-3 sm:px-6">
				{#each links as { name, href } (href)}
					<a
						{href}
						target="_blank"
						rel="noreferrer"
						class="lg-btn lg-btn-ghost justify-start"
						onclick={closeDrawer}
					>
						<span>{name}</span>
						<ExternalLink size={14} class="ml-auto opacity-60" />
					</a>
				{/each}
			</nav>
		</div>
	{/if}
</header>
