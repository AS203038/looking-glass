<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import { toasts, dismissToast, type Toast } from '$lib/toast';
	import X from '@lucide/svelte/icons/x';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import CircleX from '@lucide/svelte/icons/circle-x';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Info from '@lucide/svelte/icons/info';

	let list = $state<Toast[]>([]);
	toasts.subscribe((v) => (list = v));

	function iconFor(kind: Toast['kind']) {
		switch (kind) {
			case 'error':
				return CircleX;
			case 'warning':
				return CircleAlert;
			case 'success':
				return CircleCheck;
			case 'info':
			default:
				return Info;
		}
	}
	function badgeClassFor(kind: Toast['kind']) {
		switch (kind) {
			case 'error':
				return 'lg-badge-danger';
			case 'warning':
				return 'lg-badge-warning';
			case 'success':
				return 'lg-badge-success';
			case 'info':
			default:
				return 'lg-badge-info';
		}
	}
</script>

<div
	class="pointer-events-none fixed top-16 right-4 z-50 flex w-full max-w-sm flex-col gap-2 sm:right-6"
	aria-live="polite"
	aria-atomic="false"
>
	{#each list as t (t.id)}
		{@const Icon = iconFor(t.kind)}
		<div
			class="lg-card pointer-events-auto flex items-start gap-3 p-3 shadow-lg"
			in:fly={{ y: -8, duration: 180 }}
			out:fade={{ duration: 140 }}
			role={t.kind === 'error' ? 'alert' : 'status'}
		>
			<span class={['lg-badge mt-0.5', badgeClassFor(t.kind)].join(' ')}>
				<Icon size={14} />
			</span>
			<div class="min-w-0 flex-1">
				<p class="text-sm font-medium" style="color: var(--color-fg);">{t.title}</p>
				{#if t.message}
					<p class="mt-0.5 text-xs break-words" style="color: var(--color-fg-muted);">
						{t.message}
					</p>
				{/if}
			</div>
			<button
				type="button"
				class="lg-btn lg-btn-ghost h-7 w-7 !p-0"
				aria-label="Dismiss"
				onclick={() => dismissToast(t.id)}
			>
				<X size={14} />
			</button>
		</div>
	{/each}
</div>
