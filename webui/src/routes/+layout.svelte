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

	  Header       — sticky top: 0      (always visible)
	  <main>       — flex-1, scrolls with document
	  ResultsSheet — sticky bottom: 0   (above the dock; hidden/peek/expand)
	  CommandDock  — sticky bottom: 0   (always reachable)
	  Footer       — normal flow at the document's end

	Both sticky-bottom elements live in the same flex column. Because the
	ResultsSheet is rendered *before* the dock in source order, when both
	have `bottom: 0` they naturally stack — the dock at the very bottom,
	the sheet directly above it. The sheet's height is controlled by its
	internal state (hidden / peek / expand+px) so the picker behind it
	always has room.
-->
<div
	class="flex min-h-screen flex-col"
	style="background-color: var(--color-bg); color: var(--color-fg);"
>
	<Header />
	<main class="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-6 px-4 pt-6 pb-2 sm:px-6 sm:pt-8">
		{@render children()}
	</main>
	<ResultsSheet />
	<CommandDock />
	<Footer />
</div>

<Toasts />
