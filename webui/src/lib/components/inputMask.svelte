<script lang="ts">
  import { fade } from "svelte/transition";
  import { type Pb } from "$lib/grpc";

  import { Autocomplete, popup } from "@skeletonlabs/skeleton";
  import type {
    AutocompleteOption,
    PopupSettings,
  } from "@skeletonlabs/skeleton";
  import ExecCommand from "$lib/components/execCommand.svelte";

  export let routers: Pb.Router[] = [];

  let commands: AutocompleteOption<string>[] = [
    { value: "ping", label: "Ping" },
    { value: "traceroute", label: "Traceroute" },
    { value: "bgp_route", label: "BGP Route" },
    { value: "bgp_community", label: "BGP Community" },
    { value: "bgp_aspath_regex", label: "BGP ASPath Regex" },
  ];
  let popupSettings: PopupSettings = {
    event: "focus-click",
    target: "popupAutocomplete",
    placement: "bottom",
  };

  let autocomplete_input: string = "";
  let _cmd = "";
  let _param = "";

  export let command: string = "";
  export let parameter: string = "";

  let exec: (command: string, parameter: string) => void;
</script>

{#if Object.keys(routers).length > 0}
  <form
    class="flex flex-wrap gap-4 justify-center w-full"
    transition:fade|global
    on:submit|preventDefault={() => {
      if (_cmd == "" || _param == "" || Object.keys(routers).length == 0) {
        return;
      }
      command = _cmd;
      parameter = _param;
      exec(_cmd, _param);
    }}
  >
    <div class="w-80">
      <p class="capitalize font-medium my-2">Action:</p>
      <input
        class="input autocomplete"
        type="search"
        name="command"
        bind:value={autocomplete_input}
        on:click={() => {
          autocomplete_input = "";
          _cmd = "";
        }}
        placeholder="Search..."
        use:popup={popupSettings}
        autocomplete="off"
      />
      <div
        transition:fade|global
        class="card w-80 max-h-48 overflow-y-auto z-50"
        tabindex="-1"
        data-popup="popupAutocomplete"
      >
        <Autocomplete
          options={commands}
          bind:input={autocomplete_input}
          on:selection={(event) => {
            autocomplete_input = event.detail.label;
            _cmd = event.detail.value;
          }}
        />
      </div>
    </div>
    <div class="w-80">
      <p class="capitalize font-medium my-2">Parameter:</p>
      <input
        class="input"
        type="text"
        name="parameter"
        bind:value={_param}
        placeholder="Parameter..."
      />
    </div>
    <div transition:fade|global class="p-2 my-2 w-full">
      <button
        type="submit"
        class="btn btn-xl variant-filled-{_cmd == '' ||
        _param == '' ||
        Object.keys(routers).length == 0
          ? 'disabled'
          : 'primary'}"
        disabled={_cmd == "" ||
          _param == "" ||
          Object.keys(routers).length == 0}>Execute</button
      >
    </div>
  </form>
  <ExecCommand {routers} bind:exec />
{/if}
