<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { getEnv } from '$lib/env';
	import Header from '$lib/components/layout/Header.svelte';
	import Footer from '$lib/components/layout/Footer.svelte';
	import ResultsSheet from '$lib/components/ResultsSheet.svelte';
	import CommandDock from '$lib/components/CommandDock.svelte';
	import Toasts from '$lib/components/Toasts.svelte';

	let { children } = $props();

	const env = getEnv();
	const title = env.PUBLIC_PAGE_TITLE || env.PUBLIC_HEADER_TEXT || 'Looking Glass';

	onMount(() => {
		// Tear down the pre-hydration overlay defined in app.html.
		const overlay = document.getElementById('lg-boot');
		if (!overlay) return;
		const cleanup = () => overlay.remove();
		overlay.addEventListener('transitionend', cleanup, { once: true });
		setTimeout(cleanup, 1000); // safety net
		requestAnimationFrame(() => overlay.classList.add('lg-hide'));
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="generator" content={`AS203038/looking-glass ${env.PUBLIC_LG_VERSION}`} />
</svelte:head>

<!--
	Layout (document-scroll model, no nested overflow):

	  Header              — sticky top: 0       (always visible)
	  <main>              — flex-1, scrolls with document
	  ┌ sticky bottom-bar ──────────────────────
	  │ ResultsSheet      — hidden/peek/expand
	  │ CommandDock       — always reachable
	  └─────────────────────────────────────────
	  Footer              — normal flow at the document's end

	Why ResultsSheet + CommandDock live in ONE sticky wrapper instead of
	being individual `sticky bottom: 0` siblings:

	Two siblings both `sticky bottom: 0` overlap on the same anchor
	line, so the sheet would visually slide *over* the dock once tall
	enough — exactly the mobile regression we kept hitting. Putting them
	in a single sticky flex column makes occlusion structurally
	impossible: they stack like normal flex children (sheet above, dock
	below) and the wrapper as a whole is what's stuck to the viewport
	bottom. The sheet's max-height is computed against the live dock
	height (see `$lib/stores/sheet.ts`), so a growing dock chip strip
	just shrinks the sheet rather than pushing the dock off-screen.
-->
<div
	class="flex min-h-[100dvh] flex-col"
	style="background-color: var(--color-bg); color: var(--color-fg);"
>
	<Header />
	<main class="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-6 px-4 pt-6 pb-2 sm:px-6 sm:pt-8">
		{@render children()}
	</main>
	<div
		class="sticky bottom-0 z-20 flex flex-col"
		style="padding-bottom: env(safe-area-inset-bottom);"
	>
		<ResultsSheet />
		<CommandDock />
	</div>
	<Footer />
</div>

<Toasts />
