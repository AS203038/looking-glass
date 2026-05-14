<script lang="ts">
	/**
	 * In-component loader (matches the pre-hydration overlay aesthetic).
	 * Use for: per-router result fetching, router-list bootstrap, etc.
	 *
	 * Props let us scale the loader without re-implementing it.
	 */
	interface Props {
		label?: string;
		size?: 'sm' | 'md';
	}
	let { label = 'tracing route', size = 'md' }: Props = $props();

	// Derive sizing reactively so changing `size` at runtime is reflected.
	const hopSize = $derived(size === 'sm' ? 8 : 12);
	const trackW = $derived(size === 'sm' ? 120 : 220);
</script>

<div
	class="flex flex-col items-center gap-3 select-none"
	style="color: var(--color-fg-muted);"
	role="status"
	aria-live="polite"
>
	<div class="lg-component-track" style="--hop: {hopSize}px; --w: {trackW}px;" aria-hidden="true">
		{#each [0, 1, 2, 3, 4] as i (i)}
			<span class="lg-component-hop" style="--i: {i}"></span>
		{/each}
		<span class="lg-component-packet"></span>
	</div>
	<span class="lg-component-sub text-xs tracking-wide">{label}</span>
</div>

<style>
	.lg-component-track {
		position: relative;
		width: var(--w);
		height: calc(var(--hop) * 2.2);
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.lg-component-track::before {
		content: '';
		position: absolute;
		top: 50%;
		left: calc(var(--hop) / 2);
		right: calc(var(--hop) / 2);
		height: 2px;
		transform: translateY(-50%);
		background: var(--color-border);
		border-radius: 2px;
	}
	.lg-component-hop {
		position: relative;
		width: var(--hop);
		height: var(--hop);
		border-radius: 50%;
		background: var(--color-border);
		animation: lg-c-hop 2.4s ease-in-out infinite;
		animation-delay: calc(var(--i) * 0.48s);
		z-index: 1;
	}
	.lg-component-packet {
		position: absolute;
		top: 50%;
		left: 0;
		width: calc(var(--hop) * 0.55);
		height: calc(var(--hop) * 0.55);
		border-radius: 50%;
		background: var(--color-accent);
		transform: translate(-50%, -50%);
		animation: lg-c-packet 2.4s ease-in-out infinite;
		z-index: 2;
	}
	.lg-component-sub::after {
		content: '';
		display: inline-block;
		width: 1.2em;
		text-align: left;
		animation: lg-c-dots 1.4s steps(4, end) infinite;
	}
	@keyframes lg-c-hop {
		0%,
		40%,
		100% {
			background: var(--color-border);
			transform: scale(1);
		}
		10%,
		25% {
			background: var(--color-accent);
			transform: scale(1.35);
		}
	}
	@keyframes lg-c-packet {
		0% {
			left: 0%;
			opacity: 0;
		}
		5% {
			opacity: 1;
		}
		95% {
			opacity: 1;
		}
		100% {
			left: 100%;
			opacity: 0;
		}
	}
	@keyframes lg-c-dots {
		0% {
			content: '';
		}
		25% {
			content: '.';
		}
		50% {
			content: '..';
		}
		75%,
		100% {
			content: '...';
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.lg-component-hop {
			animation: none;
			background: var(--color-accent);
			opacity: 0.4;
		}
		.lg-component-packet {
			display: none;
		}
		.lg-component-sub::after {
			animation: none;
			content: '...';
		}
	}
</style>
