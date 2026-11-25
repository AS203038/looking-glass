<script lang="ts">
	import Sun from '@lucide/svelte/icons/sun';
	import Moon from '@lucide/svelte/icons/moon';
	import { mode, toggleMode } from '$lib/theme';
</script>

<button
	type="button"
	class="lg-btn lg-btn-ghost h-9 w-9 !p-0"
	aria-label={$mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
	title={$mode === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
	onclick={toggleMode}
>
	{#if $mode === 'dark'}
		<Sun size={18} />
	{:else}
		<Moon size={18} />
	{/if}
</button>
