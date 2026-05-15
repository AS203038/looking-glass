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
		const overlay = document.getElementById('lg-boot');
		if (!overlay) return;
		const cleanup = () => overlay.remove();
		overlay.addEventListener('transitionend', cleanup, { once: true });
		setTimeout(cleanup, 1000);
		requestAnimationFrame(() => overlay.classList.add('lg-hide'));
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="generator" content={`AS203038/looking-glass ${env.PUBLIC_LG_VERSION}`} />
</svelte:head>

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
