<script lang="ts">
	import { getEnv, parseLinks } from '$lib/env';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import Code from '@lucide/svelte/icons/code';

	const env = getEnv();
	const customLinks = parseLinks(env.PUBLIC_FOOTER_LINKS);
	const version = env.PUBLIC_LG_VERSION.split('+')[0];

	const visible =
		!!env.PUBLIC_FOOTER_LOGO || !!env.PUBLIC_FOOTER_TEXT || customLinks.length > 0 || !!version;
</script>

{#if visible}
	<footer
		class="mt-auto border-t"
		style="background-color: var(--color-bg-mantle); border-color: var(--color-border);"
	>
		<div
			class="mx-auto flex max-w-7xl flex-wrap items-center gap-3 px-4 py-3 text-sm sm:px-6"
			style="color: var(--color-fg-muted);"
		>
			<div class="flex min-w-0 items-center gap-3">
				{#if env.PUBLIC_FOOTER_LOGO}
					<a href="/"><img src={env.PUBLIC_FOOTER_LOGO} alt="Logo" class="h-7 w-auto" /></a>
				{/if}
				{#if env.PUBLIC_FOOTER_TEXT}
					<a href="/" class="truncate uppercase">{env.PUBLIC_FOOTER_TEXT}</a>
				{/if}
			</div>

			<nav class="ml-auto flex flex-wrap items-center gap-1">
				{#each customLinks as { name, href } (href)}
					<a
						{href}
						target="_blank"
						rel="noreferrer"
						class="lg-btn lg-btn-ghost h-8 !px-2.5 text-xs"
					>
						<span>{name}</span>
						<ExternalLink size={12} class="opacity-60" />
					</a>
				{/each}

				<a
					href="https://github.com/AS203038/looking-glass"
					target="_blank"
					rel="noreferrer"
					class="lg-btn lg-btn-ghost h-8 !px-2.5 text-xs"
					title="Source code on GitHub"
				>
					<Code size={12} class="opacity-60" />
					<span class="font-mono">v{version}</span>
				</a>
			</nav>
		</div>
	</footer>
{/if}
