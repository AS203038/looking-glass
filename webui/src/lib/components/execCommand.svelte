<script lang="ts">
    import { fade } from "svelte/transition";
    import { LookingGlassClient, type Pb } from "$lib/grpc";
    import { ProgressRadial, clipboard } from "@skeletonlabs/skeleton";
    import { beforeUpdate } from "svelte";
    import Icon from "@iconify/svelte";

    export let routers: Pb.Router[];

    let outputs: Record<string, string | undefined> = {};
    let fire: boolean = false;
    let _command: string;
    let _parameter: string;

    export let exec = (a: string, b: string) => {};
    function _exec(command: string, parameter: string) {
        fire = true;
        outputs = {};
        _command = command;
        _parameter = parameter;
        for (let router of routers) {
            outputs[router.id.toString()] =
                router.name + " $ " + _command + " '" + _parameter + "'\n";
            switch (command) {
                case "ping":
                    LookingGlassClient()
                        .ping(<Pb.PingRequest>{
                            routerId: router.id,
                            target: parameter,
                        })
                        .then((res) => {
                            outputs[router.id.toString()] += res.result;
                        })
                        .catch((err) => {
                            outputs[router.id.toString()] += err;
                        });
                    break;
                case "traceroute":
                    LookingGlassClient()
                        .traceroute(<Pb.TracerouteRequest>{
                            routerId: router.id,
                            target: parameter,
                        })
                        .then((res) => {
                            outputs[router.id.toString()] += res.result;
                        })
                        .catch((err) => {
                            outputs[router.id.toString()] += err;
                        });
                    break;
                case "bgp_route":
                    LookingGlassClient()
                        .bGPRoute(<Pb.BGPRouteRequest>{
                            routerId: router.id,
                            target: parameter,
                        })
                        .then((res) => {
                            outputs[router.id.toString()] += res.result;
                        })
                        .catch((err) => {
                            outputs[router.id.toString()] += err;
                        });
                    break;
                case "bgp_community":
                    LookingGlassClient()
                        .bGPCommunity(<Pb.BGPCommunityRequest>{
                            routerId: router.id,
                            community: <Pb.BGPCommunity>{
                                asn: parseInt(parameter.split(":")[0]),
                                value: parseInt(parameter.split(":")[1]),
                            },
                        })
                        .then((res) => {
                            outputs[router.id.toString()] += res.result;
                        })
                        .catch((err) => {
                            outputs[router.id.toString()] += err;
                        });
                    break;
                case "bgp_aspath_regex":
                    LookingGlassClient()
                        .bGPASPath(<Pb.BGPASPathRequest>{
                            routerId: router.id,
                            pattern: parameter,
                        })
                        .then((res) => {
                            outputs[router.id.toString()] += res.result;
                        })
                        .catch((err) => {
                            outputs[router.id.toString()] += err;
                        });
                    break;
                default:
                    break;
            }
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
                    {#if outputs[router.id.toString()] == "" || outputs[router.id.toString()] == router.name + " $ " + _command + " '" + _parameter + "'\n"}
                        <div
                            in:fade|global
                            class="text-center flex flex-col items-center w-full max-w-64"
                        >
                            <ProgressRadial />
                        </div>
                    {:else}
                        <pre
                            in:fade|global
                            class="pre text-left max-h-80 h-max"
                            data-clipboard={router.id.toString()}>{outputs[
                                router.id.toString()
                            ]}</pre>
                        <button
                            class="btn-icon variant-filled-primary absolute top-0 right-0"
                            use:clipboard={{ element: router.id.toString() }}
                        >
                            <Icon icon="ic:baseline-content-copy" />
                        </button>
                    {/if}
                </section>
            </div>
        {/if}
    {/each}
</div>
