<script lang="ts">
	import "../app.postcss";
	import { AppShell, AppBar, LightSwitch } from "@skeletonlabs/skeleton";
	import { env } from "$env/dynamic/public";
	import {
		computePosition,
		autoUpdate,
		offset,
		shift,
		flip,
		arrow,
	} from "@floating-ui/dom";
	import { storePopup, popup } from "@skeletonlabs/skeleton";
	import Icon from "@iconify/svelte";
	import { onMount } from "svelte";

	storePopup.set({ computePosition, autoUpdate, offset, shift, flip, arrow });

	// Generate iterable Link List
	export let header_links: any[] = [];
	if (env.PUBLIC_HEADER_LINKS) {
		header_links = env.PUBLIC_HEADER_LINKS.split(",").map((link) => {
			const [name, href] = link.split("|");
			return { name, href };
		});
	}

	// Generate iterable Link List
	export let footer_links: any[] = [];
	if (env.PUBLIC_FOOTER_LINKS) {
		footer_links = env.PUBLIC_FOOTER_LINKS.split(",").map((link) => {
			const [name, href] = link.split("|");
			return { name, href };
		});
	}

	// Set Page Title
	export let title =
		env.PUBLIC_PAGE_TITLE || env.PUBLIC_HEADER_TEXT || "Looking Glass NG+";

	let footer_enabled =
		env.PUBLIC_FOOTER_LINKS != "" ||
		env.PUBLIC_FOOTER_LOGO != "" ||
		env.PUBLIC_FOOTER_TEXT != "";

	onMount(() => {
		// Set Theme
		document.body.setAttribute("data-theme", env.PUBLIC_THEME);
		(function Gn(){const e=document.documentElement.classList,t=localStorage.getItem("modeUserPrefers")==="false",n=!("modeUserPrefers"in localStorage),r=window.matchMedia("(prefers-color-scheme: dark)").matches;t||n&&r?e.add("dark"):e.remove("dark")})();
	});
</script>

<svelte:head>
	<title>{title}</title>
</svelte:head>

<!-- App Shell -->
<AppShell>
	<svelte:fragment slot="header">
		<!-- App Bar -->
		<AppBar
			gridColumns="grid-cols-3"
			slotDefault="place-self-center"
			slotTrail="place-content-end"
		>
			<svelte:fragment slot="lead">
				<a class="text-xl uppercase" href="/">
					<img
						class="w-16 h-full"
						alt="Logo"
						src={env.PUBLIC_HEADER_LOGO}
					/>
				</a>
			</svelte:fragment>
			<a class="text-xl uppercase" href="/">{env.PUBLIC_HEADER_TEXT}</a>
			<svelte:fragment slot="trail">
				{#if env.PUBLIC_HEADER_LINKS}
					<button
						use:popup={{ event: "click", target: "header_links" }}
						class="btn-icon btn-sm lg:!hidden"
					>
						<Icon icon="ic:baseline-menu" class="text-xl" />
					</button>
					<span class="relative hidden lg:block space-x-2">
						{#each header_links as { name, href }}
							<a
								class="btn btn-sm variant-ghost inline"
								{href}
								target="_blank"
								rel="noreferrer"
							>
								<span>{name}</span>
							</a>
						{/each}
					</span>
					<div
						class="card p-4 w-60 shadow-xl"
						data-popup="header_links"
					>
						<div class="space-y-4">
							<nav class="list-nav">
								<ul>
									{#each header_links as { name, href }}
										<li>
											<a
												{href}
												target="_blank"
												rel="noreferrer"
											>
												<span>{name}</span>
											</a>
										</li>
									{/each}
								</ul>
							</nav>
						</div>
					</div>
				{/if}
				<div class="ml-4">
					<LightSwitch />
				</div>
			</svelte:fragment>
		</AppBar>
	</svelte:fragment>
	<!-- Page Route Content -->
	<slot />
	<svelte:fragment slot="footer">
		{#if footer_enabled}
			<!-- App Bar -->
			<AppBar
				gridColumns="grid-cols-3"
				slotDefault="place-self-center"
				slotTrail="place-content-end"
			>
				<svelte:fragment slot="lead">
					{#if env.PUBLIC_FOOTER_LOGO}
						<a class="text-xl uppercase" href="/">
							<img
								class="w-16 h-full"
								alt="Logo"
								src={env.PUBLIC_FOOTER_LOGO}
							/>
						</a>
					{/if}
				</svelte:fragment>
				{#if env.PUBLIC_FOOTER_TEXT}
					<a class="uppercase" href="/">{env.PUBLIC_FOOTER_TEXT}</a>
				{/if}
				<svelte:fragment slot="trail">
					{#if env.PUBLIC_FOOTER_LINKS}
						<button
							use:popup={{
								event: "click",
								target: "footer_links",
							}}
							class="btn-icon btn-sm lg:!hidden"
						>
							<Icon icon="ic:baseline-menu" class="text-xl" />
						</button>
						<span class="relative hidden lg:block space-x-2">
							{#each footer_links as { name, href }}
								<a
									class="btn btn-sm variant-ghost inline"
									{href}
									target="_blank"
									rel="noreferrer"
								>
									<span>{name}</span>
								</a>
							{/each}
						</span>
						<div
							class="card p-4 w-60 shadow-xl"
							data-popup="footer_links"
						>
							<div class="space-y-4">
								<nav class="list-nav">
									<ul>
										{#each footer_links as { name, href }}
											<li>
												<a
													{href}
													target="_blank"
													rel="noreferrer"
												>
													<span>{name}</span>
												</a>
											</li>
										{/each}
									</ul>
								</nav>
							</div>
						</div>
					{/if}
				</svelte:fragment>
			</AppBar>
		{/if}
	</svelte:fragment>
</AppShell>
