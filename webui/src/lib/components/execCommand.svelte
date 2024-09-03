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
      blob: Blob | undefined;
      download: boolean;
      length: number;
      ready: boolean;
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
        download: false,
        blob: undefined,
        length: 0,
        ready: false,
      };
      let res:
        | Pb.PingResponse
        | Pb.TracerouteResponse
        | Pb.BGPRouteResponse
        | Pb.BGPCommunityResponse
        | Pb.BGPASPathResponse;
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
      outputs[routerId].length = res.result.length;
      if (res.result.length >= 1024 * 1024 * 10) {
        outputs[routerId].download = true;
        outputs[routerId].blob = new Blob([res.result], { type: "text/plain" });
      } else {
        outputs[routerId].result = res.result;
      }
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
              class="text-center flex flex-col items-center w-full max-w-64"
            >
              <ProgressRadial />
            </div>
          {:else}
            {#if outputs[router.id.toString()]?.download}
              <div class="w-full flex flex-col items-center pb-4">
                <a
                  href={window.URL.createObjectURL(
                    outputs[router.id.toString()]?.blob,
                  )}
                  download="output.txt"
                  class="btn variant-filled-primary"
                >
                  Download Output ({(
                    outputs[router.id.toString()].length /
                    1024 /
                    1024
                  ).toFixed(0)} MB)
                </a>
              </div>
            {:else}
              <pre
                in:fade|global
                class="pre text-left max-h-80 h-max"
                data-clipboard={router.id.toString()}>{new TextDecoder().decode(
                  outputs[router.id.toString()]?.result,
                )}</pre>
              <button
                class="btn-icon variant-filled-primary absolute top-0 right-0"
                use:clipboard={{ element: router.id.toString() }}
              >
                <Icon icon="ic:baseline-content-copy" />
              </button>
            {/if}
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
