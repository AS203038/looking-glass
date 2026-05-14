<script lang="ts">
	/**
	 * Page composition is intentionally minimal: the picker is the only
	 * thing that scrolls inside <main>. Results live in <ResultsSheet>
	 * which is anchored above <CommandDock> at the layout level, so they
	 * never push the picker out of view — the user resizes the sheet to
	 * taste.
	 */
	import RouterPicker from '$lib/components/RouterPicker.svelte';
</script>

<RouterPicker />
