# Router Templates

Vendor support in Looking Glass is **data, not code**: every router type is a single YAML file describing how to render the right CLI command for each operation. Adding a new vendor is therefore a matter of writing a YAML file and dropping it into the loader. No Go changes, no recompilation.

This document is the authoritative reference for template authors and operators who want to override or extend shipped templates. Pair it with the existing files in [`pkg/routers/`](../pkg/routers/) — they are the best examples.

## Mental model

Each template implements the `Router` interface (`pkg/utils/interface.go`) for one vendor:

```go
type Router interface {
    Ping              (*RouterConfig, *IPNet)            ([]string, error)
    Traceroute        (*RouterConfig, *IPNet)            ([]string, error)
    BGPRoute          (*RouterConfig, *IPNet)            ([]string, error)
    BGPCommunity      (*RouterConfig, string /*comm*/)   ([]string, error)
    BGPLargeCommunity (*RouterConfig, string /*lcomm*/)  ([]string, error)
    BGPASPath         (*RouterConfig, string /*regex*/)  ([]string, error)
}
```

For YAML templates the implementation is shared: `pkg/routers/Yaml` loads the YAML, exposes the same six methods, and returns the rendered command list. Execution (SSH, output capture) is handled once in `pkg/utils.SSHExec` — the template only declares the strings to run.

A request like "ping 1.1.1.1 from router 3" therefore flows:

```
gRPC handler
   ↓
RouterInstance.Ping(target)             ← pkg/utils/interface.go
   ↓
Yaml.Ping(cfg, target)                  ← pkg/routers/yaml.go
   ↓  renders the templates in ping.ipv4 / ping.ipv6 / ping.any
[]string{"ping -n -4 -c5 -I 192.0.2.1 1.1.1.1"}
   ↓
SSHExec(cfg, cmds)                      ← pkg/utils/ssh.go
   ↓
PingResponse.Result = stdout
```

## File layout

```yaml
name: my_vendor_v1            # required, must be unique in the registry

ping:                         # optional but recommended
    any:                      # IP-family-agnostic; used if family-specific is empty
        - <cmd>
    ipv4:                     # used for IPv4 operands
        - <cmd>
        - <cmd>               # multiple commands run sequentially, output concatenated
    ipv6:
        - <cmd>

traceroute:                   # same shape as ping
    any: []
    ipv4: []
    ipv6: []

bgp:                          # BGP operations
    route:                    # bgp.route — operand: prefix/address (IPNet)
        - <cmd>
    community:                # bgp.community — operand: ASN:VALUE (string)
        - <cmd>
    largecommunity:           # bgp.largecommunity — operand: G:L1:L2 (string)
        - <cmd>
    aspath:                   # bgp.aspath — operand: sanitised regex (string)
        - <cmd>
    peer_routes:              # bgp.peer_routes — per-peer session routing lookup
        received:
            - <cmd>
        accepted:
            - <cmd>
        rejected:
            - <cmd>
        advertised:
            - <cmd>
```

Notes:

* The `name:` field is what users put in `devices[].type:` in `config.yaml`. Make it unique and stable.
* Every operation section is **optional**. A missing section means the server returns `errs.OperationUnknown` ("operation unknown") for that operation against that router type.
* `ping.any` / `traceroute.any` are the fallback when the operand's family-specific list is empty. Almost every shipped template uses family-specific lists.
* Each entry in a list is a separate command. They are executed in order; outputs are concatenated with `\n` before returning to the client.

## Template variables

Templates use Go's [`text/template`](https://pkg.go.dev/text/template) syntax. The data context is the unexported `_tpl_data` struct in `pkg/routers/yaml.go`:

```go
type _tpl_data struct {
    Cfg            *RouterConfig // the operator's device entry
    IP             *IPNet        // for ping/traceroute/bgp.route
    Community      string        // for bgp.community
    LargeCommunity string        // for bgp.largecommunity
    ASPath         string        // for bgp.aspath (already sanitised)
    PeerIP         string        // BGP peer IP address (for bgp.peer_routes)
    PeerName       string        // BGP peer session name (for bgp.peer_routes)
}
```

### Custom Template Functions

The Go template engine implements custom functions to assist with vendor-specific formatting quirks:

- `bird_community`: Translates a standard community string like `"65000:100"` into BIRD's native `(65000, 100)` tuple format. E.g. `{{bird_community .Community}}`.
- `bird_large_community`: Translates a standard large community string like `"65000:100:200"` into BIRD's native `(65000, 100, 200)` tuple format. E.g. `{{bird_large_community .LargeCommunity}}`.

Practical reference:

| Expression                | Type     | Value                                                                 | Where it applies                          |
| ------------------------- | -------- | --------------------------------------------------------------------- | ----------------------------------------- |
| `{{.Cfg.Name}}`           | string   | Device display name from `config.yaml`                                | Anywhere                                  |
| `{{.Cfg.Hostname}}`       | string   | SSH endpoint (`host[:port]`)                                          | Anywhere (rare; usually not needed)       |
| `{{.Cfg.Username}}`       | string   | SSH login                                                             | Anywhere (rare)                           |
| `{{.Cfg.VRF}}`            | string   | Routing-instance label                                                | **Every operation** — see [VRF](#vrf-handling) |
| `{{.Cfg.Location}}`       | string   | Free-form location                                                    | Anywhere (rare)                           |
| `{{.Cfg.Source4.IP}}`     | string   | IPv4 source address (defaults `127.0.0.1` if unset)                   | ping/traceroute, ipv4                     |
| `{{.Cfg.Source6.IP}}`     | string   | IPv6 source address (defaults `::1` if unset)                         | ping/traceroute, ipv6                     |
| `{{.IP.IP}}`              | string   | Operand IP / network address                                          | ping, traceroute, bgp.route               |
| `{{.IP.CIDR}}`            | string   | Prefix length                                                         | ping, traceroute, bgp.route               |
| `{{.IP.Family}}`          | string   | Literal `ipv4` / `ipv6` (lowercase)                                   | ping, traceroute, bgp.route               |
| `{{.Community}}`          | string   | `ASN:VALUE` (RFC 1997)                                                | bgp.community                             |
| `{{.LargeCommunity}}`     | string   | `GLOBAL:LOCAL1:LOCAL2` (RFC 8092)                                     | bgp.largecommunity                        |
| `{{.ASPath}}`             | string   | Sanitised regex (digits, `_`, optional `$`)                           | bgp.aspath                                |

`{{.IP.Family}}` is what makes the bundled templates emit "`ipv4`" / "`ipv6`" literally. It's especially useful for single-command `bgp.route` lookups where you can avoid a Go-template conditional. When the platform needs a *different* keyword (e.g. Arista EOS uses `ip` / `ipv6` rather than `ipv4` / `ipv6`), use a conditional:

```yaml
bgp:
  route:
    - 'show {{if eq .IP.Family "ipv4"}}ip{{else}}ipv6{{end}} bgp {{.IP.IP}} vrf {{.Cfg.VRF}}'
```

### Useful Go-template idioms

```
{{if eq .IP.Family "ipv4"}}…{{else}}…{{end}}        # family branching
{{ .Cfg.VRF | printf "%q" }}                         # double-quote a value
{{- ... -}}                                          # trim leading / trailing whitespace
```

The full `text/template` action grammar is supported. Just remember that the command is rendered to a single string, then handed to the remote shell verbatim; multi-line templates produce literal embedded newlines.

## Family selection

`ping.ipv4` / `ping.ipv6` / `ping.any` (and the same for traceroute) are selected by `_tpl()` in `pkg/routers/yaml.go`:

```go
case "ping":
    tpl = rt.Template.Ping.Any        // default
    if data.IP.IsIPv4() {
        tpl = rt.Template.Ping.IPv4   // overrides if non-nil
    } else if data.IP.IsIPv6() {
        tpl = rt.Template.Ping.IPv6
    }
```

Behaviour:

* If only `any` is provided, both families use it.
* If `ipv4` is provided but `ipv6` is not, IPv6 requests fall back to nothing (template-author bug — surfaced as `operation unknown`).
* The cleanest pattern is: define both families explicitly; use `any` only for vendors where the v4 / v6 syntax is identical.

For BGP operations there are no per-family lists. `bgp.route`'s single list is rendered against the operand's family; the other three BGP operations don't carry a family hint at all.

## VRF handling

VRF (routing-instance / virtual-router / routing-table) support is **mandatory** in templates: every bundled template threads `{{.Cfg.VRF}}` through every operation, including ping/traceroute. This lets operators reliably scope lookups to the correct forwarding context without surprises.

How each platform spells it (read the bundled template files themselves for the per-vendor chapter and verse — every shipped template's header comment documents its VRF strategy):

| Platform           | ping/traceroute                   | BGP                                      |
| ------------------ | --------------------------------- | ---------------------------------------- |
| FRRouting          | Source-IP binding (`-I`/`-s`)     | `vtysh show bgp vrf {{.Cfg.VRF}} …`      |
| Cisco IOS/IOS-XE   | `vrf {{.Cfg.VRF}}`                | `show bgp vrf {{.Cfg.VRF}} …`            |
| Arista EOS         | `vrf {{.Cfg.VRF}}`                | `show {ip|ipv6} bgp … vrf {{.Cfg.VRF}}`  |
| Juniper JunOS      | `routing-instance {{.Cfg.VRF}}`   | `table {{.Cfg.VRF}}.inet{,6}.0`          |
| Nokia SR OS        | `router {{.Cfg.VRF}}`             | `show router {{.Cfg.VRF}} bgp …`         |
| MikroTik RouterOS  | `routing-table={{.Cfg.VRF}}`      | implicit (no `vrf` column on adverts)    |

For platforms with a "default" routing instance, use the platform-native default name in `config.yaml` (`default`, `inet`, `main`, `Base`, …) — your template should not have to special-case the empty string.

## Operand types

### IP / CIDR (ping, traceroute, bgp.route)

The gRPC layer accepts a string from the client and runs it through `utils.NewIPNetFromProtobuf`:

1. If it parses as a literal IP / CIDR, use that.
2. Otherwise, run a DNS lookup and use the first returned address.
3. Empty input is rejected as `errs.IPInvalid`.

Therefore inside a template you can rely on `{{.IP.IP}}` being a single literal address (or address+prefix-length) — never a hostname.

### Standard community (bgp.community)

Always `ASN:VALUE` (two colon-separated integers). The gRPC layer constructs this string from the `BGPCommunity{asn, value}` protobuf message; the WebUI/CLI parse the user's "65000:100" form into the same shape.

### Large community (bgp.largecommunity)

Always `GLOBAL:LOCAL1:LOCAL2` (three 32-bit values, decimal, colon-separated). Built the same way as the standard community.

Vendors with no native large-community CLI knob (Nokia SR OS, notably) typically use a `large:G:L1:L2` shorthand inside their existing community query — the bundled SR OS template uses exactly that.

### AS-path regex (bgp.aspath)

Validated and normalised by `utils.SanitizeASPathRegex` **before** being rendered. The rules:

* Length must be > 0 and ≤ 30 characters.
* Only ASCII digits, `_`, and an optional trailing `$` are allowed. Everything else (parentheses, `|`, `*`, `+`, …) is rejected with `errs.ASPathMalformed`.
* A leading `_` is automatically prepended if missing.
* A trailing `_` or `$` is automatically appended if missing.

This prevents both command-injection (no characters that close quotes) and ReDoS (no repetition operators). The template just gets `{{.ASPath}}` and can interpolate it verbatim, including inside double-quoted vendor syntax (JunOS, Nokia).

## Error handling

The template-rendering layer returns one of two states:

* **Success** — list of rendered strings, returned to `SSHExec`.
* **Failure** — `errs.OperationUnknown`, surfaced to the client as "operation unknown". Caused by:
  * the requested operation has no list defined,
  * a `text/template` parse error,
  * a `text/template` execute error (e.g. referencing a nil field).

The underlying `text/template` error is logged server-side with a detailed tag (router type, op, command index, raw template) — see `tplLogTag` in `yaml.go`. RPC clients only ever see the sentinel.

After rendering, command execution failures (SSH dial, auth, exec non-zero, …) are also surfaced as coarse sentinel errors (`errs.ConnectionFailed`, `errs.AuthFailed`, `errs.ExecFailed`); see [architecture.md](./architecture.md) for the full error propagation story.

## Loading rules

Templates are loaded from two sources in this order:

1. **`$ROUTER_DIR`** (if set, at process start). Every `*.yml` / `*.yaml` file is parsed and registered. A name clash here is a panic; a name clash with an embedded template is fine and silently wins.
2. **Embedded bundle** — `pkg/routers/*.yml`, compiled into the binary. Each is registered only if no template under its name has been loaded yet; otherwise a warning is logged and the bundled copy is skipped.

So if you want to override the bundled `cisco_ios` template, drop your replacement at `$ROUTER_DIR/cisco_ios.yml`. If you want to add a new vendor that nobody else has implemented, drop it at `$ROUTER_DIR/myvendor.yml`.

Any parse error in either source **panics at startup**. Validate your template (`yamllint`, `kubectl apply --dry-run=client` on a ConfigMap, etc.) before deploying it.

## Parsers (structured output)

Beyond `name`, `ping`, `traceroute` and the `bgp` block, a template can declare an optional `parsers:` map that turns vendor command output into the typed `parsed` payload on each gRPC response (see [api.md § Structured output](./api.md#structured-output) for the wire shape). Templates that omit `parsers:` keep working exactly as before — the response simply has no structured view.

The map is keyed by operation name and each entry declares the parser to run plus, optionally, an asset name:

```yaml
parsers:
  ping:
    kind: builtin
  traceroute:
    kind: builtin
  bgp.summary:
    kind: native_json
    schema: frr_bgp_summary_v1
  bgp.route:
    kind: textfsm
    template: arista_eos_show_bgp_paths
```

Valid keys: `ping`, `traceroute`, `bgp.summary`, `bgp.route`, `bgp.community`, `bgp.largecommunity`, `bgp.aspath`. Unknown keys are ignored; unknown `kind:` values fall back to `raw` with a warning in the log.

### Parser kinds

| `kind`        | When to use                                                                                                                                                                                                                                  |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `raw`         | Default. No parsing; the response only carries the verbatim bytes. Pick this when you don't have a template yet — it's strictly safer than shipping a wrong parser.                                                                          |
| `textfsm`     | The recommended default. Run a TextFSM template (compatible with the `networktocode/ntc-templates` ecosystem) on plain CLI output, server-side. Cheap on the router because it's just running its normal `show` command.                     |
| `native_json` | Tell the template to ask the router for JSON directly (`vtysh -c '... json'`, `\| display json`, `\| json`). **Opt-in only** — see the policy box below. Currently routinely-recommended only for FRR, where it's essentially free.            |
| `builtin`     | A small, hand-rolled Go parser. Used for trivially-structured outputs whose format has been stable for decades (Linux iputils `ping` and `traceroute`). New `builtin` handlers should be added sparingly — TextFSM is the right tool for nearly everything else. |

### Asset fields

* `template:` — the **TextFSM template name** (without extension). The dispatcher looks for it under `$ROUTER_DIR/textfsm/<name>.textfsm` first, then under the bundled embedded assets. Lookup tries `.textfsm` then `.tfsm`. Used only for `kind: textfsm`.
* `schema:` — the **native-JSON schema selector** (e.g. `frr_bgp_route_v1`, `frr_bgp_summary_v1`). The dispatcher consults the schema name to pick the right Go decoder. Used only for `kind: native_json`.

### Policy: router CPU is the bottleneck

Looking Glass parses on its own machine, not on the router. The default for every vendor is plain CLI → TextFSM. JSON pipelines on JunOS (`| display json`), Arista (`| json`) and Cisco IOS-XE (`| json`) re-serialise text the box has already rendered and typically cost **1.5–3× the CPU and 2–4× the byte volume** on large outputs. Pick `native_json` only when you have evidence that the router-side cost is low.

The one routinely-recommended exception is **FRRouting**: FRR holds the RIB as structured data in its userland daemon, so `vtysh -c '... json'` is essentially a `memcpy`. The shipped FRR template therefore uses `native_json` for every BGP op.

### How a parser failure is reported

* When the parser succeeds, the response's `parse_status` is `PARSE_STATUS_OK` and `parsed` is populated. Clients render the structured view; `result` is still available as a fallback.
* When the parser runs but produces nothing useful (vendor output drift, malformed JSON, …), `parse_status` is `PARSE_STATUS_PARSE_FAILED`. Server-side logs include the parser name and op for triage. The request **does not fail** — `result` is still returned and the WebUI/CLI falls back to the raw view.
* When `kind: textfsm` is declared but `template:` is missing or points to a non-existent asset, `parse_status` is `PARSE_STATUS_TEMPLATE_MISSING`. Same fallback behaviour.

This makes structured output strictly additive: a broken parser declaration can only cost you the structured view, never the raw output.

### Authoring a TextFSM template

Templates live at `pkg/routers/parse/textfsm/<name>.textfsm` in the bundled set, or `$ROUTER_DIR/textfsm/<name>.textfsm` for operator overrides. They use the standard TextFSM grammar — networktocode/ntc-templates is a good reference. The Go parser backend is [`sirikothe/gotextfsm`](https://github.com/sirikothe/gotextfsm).

The Go side knows how to project records into the typed payloads by **column name** (case-insensitive). Per operation:

* `ping` → `target`, `source`, `packets_sent`, `packets_received`, `loss_pct`, `rtt_min_ms`, `rtt_avg_ms`, `rtt_max_ms`, `rtt_mdev_ms`.
* `traceroute` → one record per hop with `ttl`, `ip`, `hostname`, `rtt_ms`, `asn`; plus optional `target` / `source` Filldown'd across rows.
* `bgp.summary` → one record per peer with `peer_ip`, `peer_asn`, `description`, `state`, `state_detail`, `uptime` (mapped to protobuf `uptime_seconds`), `prefixes_received`, `prefixes_accepted`, `prefixes_sent`, `address_family`; plus optional `local_asn` / `router_id` Filldown'd.
* `bgp.route` / `bgp.community` / `bgp.largecommunity` / `bgp.aspath` → one record per BGP path with `prefix`, `nexthop`, `as_path` (List of ASNs or single space-joined string), `origin`, `med`, `local_pref`, `communities` (List of `ASN:VAL` strings), `large_communities` (List), `best` (string; recognised truthy values include `true`, `1`, `yes`, `>`, `*`), `peer_ip`, `peer_asn`, `age` (mapped to protobuf `age_seconds`).

Columns the template does not emit simply land at their proto zero value — nothing in the parser is mandatory.

Caveat: gotextfsm treats the first blank line after the `Value` declarations as the end of the Values block. **Don't put blank lines between the header comments and the first `Value` line**; the first blank line should come after the last `Value`.

### Authoring native-JSON schema support

`native_json` schema names map to Go decoders in `pkg/routers/parse/json.go`. To add a new schema you need a small Go change (define the input shape, add a case in `JSONParser.Parse`, project onto the typed payload). The intent is that operators need to do this very rarely — TextFSM should be the path of least resistance for adding new vendors.

## Authoring a new template

Pick a vendor that isn't shipped (let's pretend we're adding OpenBSD `bgpd`):

1. **Choose a unique name** — `openbsd_bgpd`. This is the value operators put in `devices[].type:`.
2. **Identify the platform's command surface.** For OpenBSD that's `ping`, `ping6`, `traceroute`, `traceroute6`, and `bgpctl`.
3. **Write the YAML.** Start from a copy of the closest existing template (`frrouting.yml` is a good base for Unix-style hosts).
4. **Drop the file at `$ROUTER_DIR/openbsd_bgpd.yml`.**
5. **Add a device** in `config.yaml` using `type: openbsd_bgpd`.
6. **Run the server.** The startup log should show:
   ```
   NOTICE: Loading routers from /etc/looking-glass/routers
   NOTICE: Router openbsd_bgpd (/etc/looking-glass/routers/openbsd_bgpd.yml) registered
   ```
7. **Exercise each operation** via the WebUI or `lg-cli` and adjust commands until they match the platform's native CLI output.
8. **Optional but encouraged**: open a PR to add the template to `pkg/routers/` so everyone benefits.

### Walked-through example

```yaml
# /etc/looking-glass/routers/openbsd_bgpd.yml
name: openbsd_bgpd

ping:
    ipv4:
        - /sbin/ping -c5 -V {{.Cfg.VRF}} -I {{.Cfg.Source4.IP}} {{.IP.IP}}
    ipv6:
        - /sbin/ping6 -c5 -V {{.Cfg.VRF}} -I {{.Cfg.Source6.IP}} {{.IP.IP}}

traceroute:
    ipv4:
        - /usr/sbin/traceroute -V {{.Cfg.VRF}} -s {{.Cfg.Source4.IP}} {{.IP.IP}}
    ipv6:
        - /usr/sbin/traceroute6 -V {{.Cfg.VRF}} -s {{.Cfg.Source6.IP}} {{.IP.IP}}

bgp:
    route:
        - bgpctl -n {{.Cfg.VRF}} show rib {{.IP.IP}}
    community:
        - bgpctl -n {{.Cfg.VRF}} show rib community {{.Community}}
    largecommunity:
        - bgpctl -n {{.Cfg.VRF}} show rib large-community {{.LargeCommunity}}
    aspath:
        - bgpctl -n {{.Cfg.VRF}} show rib as {{.ASPath}}
```

The example shows the three things every template tends to do:

1. **Split per family** for ping/traceroute (binary name differs).
2. **Thread `{{.Cfg.VRF}}` through every command** (here as `bgpd`'s `-n` routing-domain selector).
3. **Use a single `bgp.route` command** with the operand's family handled by the vendor's CLI.

## Multi-command operations

Each entry in a list is a separate command. For example, to query both IPv4 and IPv6 unicast tables on Cisco IOS (because `community` has no operand-derived family):

```yaml
bgp:
  community:
    - show bgp vrf {{.Cfg.VRF}} ipv4 unicast community {{.Community}}
    - show bgp vrf {{.Cfg.VRF}} ipv6 unicast community {{.Community}}
```

Both commands run in separate SSH sessions (over the same TCP connection) and the outputs are joined with `\n` in the order listed. If any command fails, the whole operation fails — there is no partial-success path.

## Security considerations

The template body is operator-controlled, so the threat model is **not** "untrusted user injects shell metacharacters". But it is:

* **Operator typos** that produce unexpected commands.
* **Operand-derived injection** — what happens if `{{.IP.IP}}` contained a shell metacharacter?

The codebase prevents the latter by parsing every operand into a strict type:

* IP / CIDR — parsed by `net.ParseIP` / `net.ParseCIDR`. Output is always `[0-9a-f.:/]+`.
* Community / large-community — built from integer fields, so the rendered string is always `\d+:\d+` / `\d+:\d+:\d+`.
* AS-path regex — `SanitizeASPathRegex` allow-listing.

So **the only way to get shell metacharacters into a rendered command is via the template itself or the operator's config fields** (e.g. a `vrf:` containing a literal `; rm -rf /`). The realistic guidance is therefore:

1. Don't put untrusted data in `config.yaml`. (It contains credentials anyway; protect it accordingly.)
2. Use read-only router accounts.
3. Test templates against a non-production device before deploying widely.

### Privilege & command authorization on the device side

The LG only ever runs a small, fixed set of commands against each vendor (the contents of the `ping` / `traceroute` / `bgp.*` blocks in the template). Every bundled template documents that set explicitly in its header comment. Operators should configure a **bespoke, least-privilege role/class/profile** on each device that permits exactly those commands and denies everything else — not a generic `network-admin` / `privilege 15` account.

Copy-pasteable per-vendor role definitions (Arista `role …`, Cisco `parser view` / privilege levels, JunOS `login class`, SR OS `profile`, RouterOS `/user group`, FRR sshd `ForceCommand` wrapper) live in [router-hardening.md](./router-hardening.md). When you author a new template, update `router-hardening.md` in the same PR — the two documents are a contract:

* `router-templates.md` defines which CLI commands the LG sends.
* `router-hardening.md` defines which commands the device permits.

If those two ever drift, either the LG breaks (template adds a command the role denies) or the security boundary leaks (role permits more than the template needs).

## Style guide

When contributing a template to the bundled set:

* **File name** `<vendor>_<model>.yml` (e.g. `cisco_ios.yml`, `juniper_junos.yml`).
* **`name:`** matches the file's basename (so users have an easy mapping from filesystem to config field).
* **Five-section header comment** documenting:
  1. Target platform and version range.
  2. Login surface (CLI mode, any prerequisites).
  3. Notable design choices (per-family splits, VRF spellings, vendor-specific tokenisation quirks).
* **Order**: `ping → traceroute → bgp.route → bgp.community → bgp.largecommunity → bgp.aspath`. Matches the order the gRPC service exposes operations.
* **Family handling**: prefer per-family lists for ping/traceroute; rely on `{{.IP.Family}}` (or a `{{if eq .IP.Family …}}` conditional) for `bgp.route`; fan out to both families for the three operand-less BGP operations.
* **VRF**: include `{{.Cfg.VRF}}` in every command. Document the spelling in the header comment.

## Testing a new template

There is no automated test harness for vendor templates yet. The practical workflow is:

1. **Render in isolation.** A short Go snippet:
   ```go
   y := &routers.Yaml{Path: "x"}
   yaml.Unmarshal(file, &y.Template)
   cmds, _ := y.Ping(&utils.RouterConfig{
       Source4: must(utils.NewIPNET("192.0.2.1")),
       VRF:     "default",
   }, must(utils.NewIPNET("1.1.1.1")))
   fmt.Println(cmds)
   ```
2. **Smoke against the device.** Copy the rendered command to your SSH session and verify the router accepts and produces useful output.
3. **Add a test device** in `config.yaml` and run the server with `ROUTER_DIR` pointing at your work-in-progress template. Use `lg-cli ping http://localhost:8080 1 1.1.1.1` to exercise it end-to-end.

We welcome contributions of a proper test harness — see [development.md](./development.md).

## Troubleshooting

| Symptom                                            | Likely cause / fix                                                                          |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Process panics on startup with `Unmarshal file`    | Malformed YAML; run `yamllint`.                                                              |
| `Router name cannot be empty` panic                | Missing `name:` field.                                                                       |
| `Router foo already registered` warning            | Two templates share the same `name:`. External (ROUTER_DIR) wins, embedded is skipped.       |
| RPC returns `operation unknown`                    | Template doesn't define a list for that op, OR a parse/execute error in the template body.  |
| Output looks empty but command should produce data | Did you forget to include the family-specific line? Check `_tpl` family selection rules.    |
| `template: x: function "foo" not defined`          | Used a sprig-style function that `text/template` doesn't ship; stick to built-ins.          |
| Quotes broken on JunOS / Nokia                     | Wrap regex operands in `"…"` literally in the template — see `juniper_junos.yml`.            |

## Reference: shipped templates

| Template            | File                                       | Notes                                                                     |
| ------------------- | ------------------------------------------ | ------------------------------------------------------------------------- |
| `frrouting`         | `pkg/routers/frrouting.yml`                | Unix `ping`/`traceroute`; `vtysh -c` for BGP. Best base for Linux hosts.  |
| `cisco_ios`         | `pkg/routers/cisco_ios.yml`                | Uses `{{.IP.Family}}` directly for `bgp.route`.                            |
| `arista_eos`        | `pkg/routers/arista_eos.yml`               | EOS wants `ip`/`ipv6` before `bgp`, not `ipv4`/`ipv6` — uses a conditional. |
| `juniper_junos`     | `pkg/routers/juniper_junos.yml`            | Per-VRF tables (`<VRF>.inet.0`); `aspath-regex` requires double-quotes.   |
| `nokia_sros`        | `pkg/routers/nokia_sros.yml`               | Single `bgp.route` command (SR OS auto-detects family); large-community via `large:` shorthand. |
| `mikrotik_routeros` | `pkg/routers/mikrotik_routeros.yml`        | RouterOS 7 only; RouterOS 6 lacks the columns these queries filter on.    |

Read them. They are the best examples of the patterns described here, and each one's header comment explains the vendor-specific quirks.
