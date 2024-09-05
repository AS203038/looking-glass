<script lang="ts">
  import { fade } from "svelte/transition";
  import { LookingGlassClient, type Pb } from "$lib/grpc";
  import { ProgressRadial, clipboard } from "@skeletonlabs/skeleton";
  import { beforeUpdate } from "svelte";
  import Icon from "@iconify/svelte";

  export let routers: Pb.Router[];

  let outputs: Record<
    string,
    {
      result: Uint8Array | undefined;
      timestamp: Date | undefined;
      length: number;
      ready: boolean;
      pages: Uint8Array[];
      currentPage: number;
      pageSize: number;
    }
  > = {};
  let fire: boolean = false;
  let _command: string;
  let _parameter: string;

  export let exec = (a: string, b: string) => {};

  async function _exec(command: string, parameter: string) {
    fire = true;
    outputs = {};
    _command = command;
    _parameter = parameter;
    for (let router of routers) {
      const routerId = router.id.toString();
      outputs[routerId] = {
        result: undefined,
        timestamp: undefined,
        length: 0,
        ready: false,
        pages: [],
        currentPage: 0,
        pageSize: 1024 * 1024 * 1, // 1MB per page
      };
      let res:
        | Pb.PingResponse
        | Pb.TracerouteResponse
        | Pb.BGPRouteResponse
        | Pb.BGPCommunityResponse
        | Pb.BGPASPathResponse;
      try {
        switch (command) {
          case "ping":
            res = await LookingGlassClient().ping(<Pb.PingRequest>{
              routerId: router.id,
              target: parameter,
            });
            break;
          case "traceroute":
            res = await LookingGlassClient().traceroute(<Pb.TracerouteRequest>{
              routerId: router.id,
              target: parameter,
            });
            break;
          case "bgp_route":
            res = await LookingGlassClient().bGPRoute(<Pb.BGPRouteRequest>{
              routerId: router.id,
              target: parameter,
            });
            break;
          case "bgp_community":
            res = await LookingGlassClient().bGPCommunity(<
              Pb.BGPCommunityRequest
            >{
              routerId: router.id,
              community: <Pb.BGPCommunity>{
                asn: parseInt(parameter.split(":")[0]),
                value: parseInt(parameter.split(":")[1]),
              },
            });
            break;
          case "bgp_aspath_regex":
            res = await LookingGlassClient().bGPASPath(<Pb.BGPASPathRequest>{
              routerId: router.id,
              pattern: parameter,
            });
            break;
          default:
            console.log("Unknown command");
            return;
        }
      } catch (e) {
        outputs[routerId].result = new TextEncoder().encode(
          `Error: ${e.message}`,
        );
        outputs[routerId].timestamp = new Date();
        outputs[routerId].ready = true;
        continue;
      }
      outputs[routerId].length = res.result.length;
      if (res.result.length > outputs[routerId].pageSize) {
        // Split result into pages
        for (let i = 0; i < res.result.length; ) {
          let end = i + outputs[routerId].pageSize;
          if (end < res.result.length) {
            // Find the next newline character after the pageSize
            while (end < res.result.length && res.result[end] !== 10) {
              // 10 is the ASCII code for newline
              end++;
            }
            end++; // Include the newline character in the chunk
          }
          outputs[routerId].pages.push(res.result.slice(i, end));
          i = end;
        }
      } else {
        outputs[routerId].result = res.result;
      }
      // }
      outputs[routerId].timestamp = new Date(
        parseInt(res.timestamp.seconds.toString()) * 1000,
      );
      outputs[routerId].ready = true;
    }
  }
  $: exec = _exec;

  beforeUpdate(() => {
    let _out = outputs;
    outputs = {};
    for (let router of routers) {
      if (_out[router.id.toString()] !== undefined) {
        outputs[router.id.toString()] = _out[router.id.toString()];
      }
    }
  });

  function nextPage(routerId: string) {
    if (outputs[routerId].currentPage < outputs[routerId].pages.length - 1) {
      outputs[routerId].currentPage++;
    }
  }

  function prevPage(routerId: string) {
    if (outputs[routerId].currentPage > 0) {
      outputs[routerId].currentPage--;
    }
  }

</script>

<div class="flex flex-wrap justify-evenly gap-4 mt-2">
  {#each routers as router}
    {#if outputs[router.id.toString()] !== undefined}
      <div transition:fade|global class="card text-left p-4">
        <header class="card-header">
          <p class="font-medium">Router: {router.name}</p>
          <p class="capitalize">Location: {router.location}</p>
        </header>
        <section class="p-4 relative">
          {#if outputs[router.id.toString()].ready === false}
            <div
              in:fade|global
              class="text-center flex flex-col items-center max-h-80 h-max max-w-3xl w-full"
            >
              <ProgressRadial />
            </div>
          {:else}
            {#if outputs[router.id.toString()]?.pages.length === 0}
              <pre
                in:fade|global
                class="pre text-left max-h-80 h-max max-w-3xl w-full overflow-scroll"
                data-clipboard={router.id.toString()}>{new TextDecoder().decode(
                  outputs[router.id.toString()].result,
                )}</pre>
            {:else}
              <pre
                in:fade|global
                class="pre text-left max-h-80 h-max max-w-3xl w-full overflow-scroll"
                data-clipboard={router.id.toString()}>{new TextDecoder().decode(
                  outputs[router.id.toString()].pages[
                    outputs[router.id.toString()].currentPage
                  ],
                )}</pre>
              <div class="flex justify-between mt-2">
                <button
                  class="btn variant-filled-primary"
                  on:click={() => prevPage(router.id.toString())}
                  disabled={outputs[router.id.toString()].currentPage === 0}
                >
                  Previous
                </button>
                <span>
                  Page {outputs[router.id.toString()].currentPage + 1} of {outputs[
                    router.id.toString()
                  ].pages.length}
                </span>
                <button
                  class="btn variant-filled-primary"
                  on:click={() => nextPage(router.id.toString())}
                  disabled={outputs[router.id.toString()].currentPage ===
                    outputs[router.id.toString()].pages.length - 1}
                >
                  Next
                </button>
              </div>
            {/if}
            <button
              class="btn-icon variant-filled-primary absolute top-0 right-0"
              use:clipboard={{ element: router.id.toString() }}
            >
              <Icon icon="ic:baseline-content-copy" />
            </button>
            <pre class="text-right text-xs">
                Timestamp: {outputs[
                router.id.toString()
              ]?.timestamp.toISOString()}
            </pre>
          {/if}
        </section>
      </div>
    {/if}
  {/each}
</div>
