<script lang="ts">
  import { fade } from "svelte/transition";
  import ListRouters from "$lib/components/listRouters.svelte";
  import InputMask from "$lib/components/inputMask.svelte";
  import type { Pb } from "$lib/grpc";
  import Icon from "@iconify/svelte";

  let routers: Pb.Router[];
  let error: any = false;

  function Error(e: any) {
    error = {
      title: e.title,
      message: e.message,
    };
  }
</script>

<div
  class="container h-full mx-auto flex flex-wrap justify-center items-center"
>
  <div
    class="space-y-2 text-center flex flex-col items-center w-full max-w-screen"
  >
    {#if error !== false}
      <aside class="alert variant-filled-error" transition:fade|global>
        <!-- Icon -->
        <Icon icon="ic:baseline-error" class="text-4xl" />
        <!-- Message -->
        <div class="alert-message">
          <h3 class="h3">{error.title}</h3>
          <p>{error.message}</p>
        </div>
        <!-- Actions -->
        <div class="alert-actions">
          <button
            class="btn-icon variant-filled"
            on:click={() => (error = false)}
          >
            <Icon icon="ic:baseline-close" />
          </button>
        </div>
      </aside>
    {/if}
    <ListRouters bind:SelectedRouters={routers} {Error} />
    <InputMask {routers} />
  </div>
</div>
