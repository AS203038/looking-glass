<script lang="ts">
  import { onMount } from "svelte";
  import { fade } from "svelte/transition";
  import { LookingGlassClient, type Pb } from "$lib/grpc";
  import { ProgressRadial } from "@skeletonlabs/skeleton";
  import { Accordion, AccordionItem } from "@skeletonlabs/skeleton";
  import { popup } from "@skeletonlabs/skeleton";
  import Icon from "@iconify/svelte";
  import { env } from "$env/dynamic/public";

  export let routers: Pb.Router[] = [];
  export let SelectedRouters: Pb.Router[] = [];
  export let Error: (e: any) => void;

  onMount(async () => {
    getRouters(1);
  });

  function getRouters(page: number) {
    LookingGlassClient()
      .getRouters(<Pb.GetRoutersRequest>{
        limit: 10,
        pageToken: page,
      })
      .then((res: Pb.GetRoutersResponse) => {
        routers = routers.concat(res.routers);
        if (res.nextPage) {
          getRouters(res.nextPage);
        }
      })
      .catch((err: any) => {
        Error({
          title: "Error fetching routers",
          message: err,
        });
        console.log(err);
      });
  }

  function selectRouter(rt: Pb.Router) {
    if (SelectedRouters.indexOf(rt) > -1) {
      SelectedRouters = SelectedRouters.filter((r) => r.id !== rt.id);
    } else {
      SelectedRouters = [...SelectedRouters, rt];
    }
  }

  function getRouterLocations() {
    let locations = [];
    for (let rt of routers) {
      if (locations.indexOf(rt.location) === -1) {
        locations.push(rt.location);
      }
    }
    return locations;
  }

  function getRouterByLocation(location: string) {
    return routers.filter((rt) => rt.location === location);
  }
</script>

<div in:fade|global>
  {#if routers.length === 0}
    <p><ProgressRadial /></p>
  {:else}
    <p class="capitalize font-medium mb-2">Available Routers:</p>
    {#if routers.length <= parseInt(env.PUBLIC_RT_LIST_MAX || "4")}
      <div class="flex flex-wrap justify-center gap-4">
        {#each routers as rt}
          {#if rt.health.healthy === true}
            <button
              in:fade|global
              class="btn {SelectedRouters.indexOf(rt) > -1
                ? 'btn-xl variant-filled'
                : 'btn-lg variant-soft'}"
              on:click={() => selectRouter(rt)}
              on:keypress
            >
              {#if SelectedRouters.indexOf(rt) > -1}
                <Icon icon="ic:baseline-check" />
              {/if}
              <p class="font-medium">{rt.name}</p>
              <p class="capitalize">{rt.location}</p>
            </button>
          {:else}
            <button
              in:fade|global
              class="btn {SelectedRouters.indexOf(rt) > -1
                ? 'btn-xl variant-filled'
                : 'btn-lg variant-soft'}"
              on:click={() => false}
              on:keypress
              disabled
              use:popup={{
                event: "hover",
                target: "rt_" + rt.id,
              }}
            >
              <p class="font-medium">{rt.name}</p>
              <p class="capitalize">{rt.location}</p>
            </button>
            <div
              class="card p-4 variant-filled-secondary"
              data-popup={"rt_" + rt.id}
            >
              <p>
                Health Check failed at {new Date(
                  parseInt(rt.health.timestamp.seconds.toString()) * 1000,
                ).toISOString()}
              </p>
            </div>
          {/if}
        {/each}
      </div>
    {:else}
      <Accordion class="card p-4 text-token">
        {#each getRouterLocations() as location}
          <AccordionItem open={getRouterLocations().indexOf(location) === 0}>
            <svelte:fragment slot="lead"
              ><Icon icon="ic:round-pin-drop" /></svelte:fragment
            >
            <svelte:fragment slot="summary"
              ><p class="font-bold capitalize">
                {location}
              </p></svelte:fragment
            >
            <svelte:fragment slot="content">
              <div class="flex flex-wrap justify-center gap-4">
                {#each getRouterByLocation(location) as rt}
                  {#if rt.health.healthy === true}
                    <button
                      in:fade|global
                      class="btn {SelectedRouters.indexOf(rt) > -1
                        ? 'btn-xl variant-filled'
                        : 'btn-lg variant-soft'}"
                      on:click={() => selectRouter(rt)}
                      on:keypress
                    >
                      {#if SelectedRouters.indexOf(rt) > -1}
                        <Icon icon="ic:baseline-check" />
                      {/if}
                      <p class="font-medium">{rt.name}</p>
                    </button>
                  {:else}
                    <button
                      in:fade|global
                      class="btn {SelectedRouters.indexOf(rt) > -1
                        ? 'btn-xl variant-filled'
                        : 'btn-lg variant-soft'}"
                      on:click={() => false}
                      on:keypress
                      disabled
                      use:popup={{
                        event: "hover",
                        target: "rt_" + rt.id,
                      }}
                    >
                      <p class="font-medium">{rt.name}</p>
                    </button>
                    <div
                      class="card p-4 variant-filled-secondary"
                      data-popup={"rt_" + rt.id}
                    >
                      <p>
                        Health Check failed at {new Date(
                          parseInt(rt.health.timestamp.seconds.toString()) *
                            1000,
                        ).toISOString()}
                      </p>
                    </div>
                  {/if}
                {/each}
              </div>
            </svelte:fragment>
          </AccordionItem>
        {/each}
      </Accordion>
    {/if}
  {/if}
</div>
