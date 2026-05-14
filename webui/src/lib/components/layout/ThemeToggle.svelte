<script lang="ts">
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';
	import { mode, toggleMode } from '$lib/theme';

	let current = $state<'light' | 'dark'>('dark');
	mode.subscribe((m) => (current = m));
</script>

<button
	type="button"
	class="lg-btn lg-btn-ghost h-9 w-9 !p-0"
	aria-label={current === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
	title={current === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
	onclick={toggleMode}
>
	{#if current === 'dark'}
		<Sun size={18} />
	{:else}
		<Moon size={18} />
	{/if}
</button>
