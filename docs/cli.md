# `lg-cli` Reference

`lg-cli` is the command-line client for Looking Glass instances.
It speaks the same ConnectRPC API as the WebUI, supports the
public-index registry for instance lookup, and ships three output
modes for both humans and scripts.

The binary is built from `cmd/cli/` and released alongside the
server. Build locally with `make build-cli`.

## Synopsis

```
lg-cli [global flags] <command> [args...]
```

The single-source help (`lg-cli --help`) is always authoritative —
this page is the longer-form companion.

## Global flags

| Flag                | Env var          | Default                                                                          | Notes                                                                                              |
| ------------------- | ---------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `--index <url>`     | `LG_INDEX_URL`   | `https://raw.githubusercontent.com/AS203038/looking-glass/main/public_index.yaml` | Source of the public-index registry.                                                                |
| `-o, --output FMT`  | `LG_OUTPUT`      | `pretty`                                                                          | One of `pretty | json | raw`.                                                                       |
| `--timeout DUR`     | `LG_TIMEOUT`     | `60s`                                                                            | Per-invocation timeout. Go duration syntax (`90s`, `2m`, `5m30s`).                                  |
| `--no-color`        | `LG_NO_COLOR` / `NO_COLOR` | (auto) when set                                                          | Disable ANSI colour in pretty output. Also auto-off when stdout isn't a TTY.                        |
| `-q, --quiet`       | —                | off                                                                              | Suppress the trailing `ts:` footer in pretty output.                                                 |
| `-v, --verbose`     | —                | off                                                                              | Diagnostic lines to stderr.                                                                          |
| `-h, --help`        | —                | —                                                                                | Show help. Works at every level (`lg-cli`, `lg-cli bgp`, `lg-cli bgp route --help`).               |

`--quiet` and `--verbose` are mutually exclusive.

## Argument resolution

### Instance argument

Most subcommands take an instance as their first positional
argument. It can be:

1. **A public-index name** — case-insensitive substring match
   against `index[].name` in `public_index.yaml`
   (e.g. `as203038`, `qux`).
2. **An ASN** — with or without the `AS` prefix; matched against
   `index[].asn` (e.g. `203038`, `AS203038`).
3. **A full URL** — anything starting with `http://` or `https://`
   is used verbatim and skips the index fetch entirely
   (e.g. `http://localhost:8080`, `https://lg.example.net`).

URLs are the escape hatch for local / private deployments. The
public-index lookup has its own 15s sub-budget so a slow or
unreachable index can never eat into the RPC budget.

### Router argument

Subcommands that target a single router (`ping`, `traceroute`,
`bgp …`) take a router argument. It can be:

1. **A numeric ID** — the `id` from `GetRouters` (e.g. `1`, `3`).
   This is the fast path; no extra round-trip is needed.
2. **A case-insensitive substring of the router's name** — exact
   match first, then substring. Ambiguous substring matches are
   rejected with a list of candidates:
   ```
   Error: router "rt" is ambiguous; matches 2 routers: 1:rt1.sto1.se, 2:rt2.sto1.se
   ```

Use IDs in scripts; use names interactively.

## Commands

### `instances`

```
lg-cli instances
```

Lists the public-index entries. No instance argument required.

### `info <instance>`

```
lg-cli info as203038
```

Calls `GetInfo` and prints the server hostname and version.

### `routers <instance>`

```
lg-cli routers as203038
```

Pages through `GetRouters` and prints the catalogue as a table:

```
ID  HEALTH   NAME             LOCATION
 1  ✓        rt1.sto1.se      Stockholm, Sweden
 2  ✓        rt2.sto1.se      Stockholm, Sweden
 3  ✗        rt1.lon1.uk      London, UK
```

The health glyph is `✓` (healthy), `✗` (unhealthy), or `?`
(never probed yet).

### `ping <instance> <router> <target>`

```
lg-cli ping as203038 1 1.1.1.1
lg-cli ping as203038 rt1.sto1 8.8.8.8
lg-cli ping http://localhost:8080 rt 1.1.1.1
```

Calls `Ping`. `<target>` may be an IPv4 or IPv6 address; the
server-side `NewIPNetFromProtobuf` will also DNS-resolve a
hostname. The server's vendor template picks the right family.

### `traceroute <instance> <router> <target>` (alias `trace`)

Same as `ping`, but for `Traceroute`.

### `bgp summary <instance> <router>`

```
lg-cli bgp summary as203038 1
```

Calls `BGPSummary`. Prints the raw neighbour-summary output.

### `bgp route <instance> <router> <prefix>`

```
lg-cli bgp route as203038 1 8.8.8.0/24
lg-cli bgp route as203038 1 2001:db8::/32
```

Calls `BGPRoute`. `<prefix>` may be either family, with or without
a mask. The server picks the right family-specific template.

### `bgp community <instance> <router> <community>`

```
lg-cli bgp community as203038 1 65000:100        # RFC 1997 standard
lg-cli bgp community as203038 1 214503:8:3607    # RFC 8092 large
```

The format is auto-detected from the number of colon-separated
fields:

* Two fields → `BGPCommunity` (standard).
* Three fields → `BGPLargeCommunity` (large).
* Anything else → error.

### `bgp aspath <instance> <router> <regex>` (alias `as-path`)

```
lg-cli bgp aspath as203038 1 65000
lg-cli bgp aspath as203038 1 _65000_
```

Calls `BGPASPath`. The regex is forwarded verbatim; the server's
`SanitizeASPathRegex` enforces the digit/underscore allow-list.

### `version`

```
lg-cli version
```

Prints the CLI version (from build-time `-X main.Version`). Note
this is the *client* version; use `lg-cli info <instance>` to
read the server version.

### `completion <shell>`

```
lg-cli completion bash > /etc/bash_completion.d/lg-cli
lg-cli completion zsh  > "${fpath[1]}/_lg-cli"
lg-cli completion fish > ~/.config/fish/completions/lg-cli.fish
lg-cli completion powershell > lg-cli.ps1
```

Cobra-generated shell completion for bash, zsh, fish, and
powershell.

## Output modes

The output mode is set via `-o`/`--output` or `LG_OUTPUT`. All
three modes are supported by every command that produces a
result.

### `pretty` (default)

* ANSI colour on a TTY, plain text otherwise.
* For operation results: prints the raw router output to stdout,
  followed by a dim `ts: <time>` footer to **stderr** so that
  redirecting stdout to a file produces a clean result.
* `--quiet` suppresses the footer entirely.
* `--no-color` disables ANSI even on a TTY.

```bash
lg-cli ping as203038 1 1.1.1.1 > result.txt    # result.txt is just router output
lg-cli ping as203038 1 1.1.1.1 2>/dev/null     # no footer, no colour, just output
```

### `json`

Machine-readable JSON shape — exact fields depend on the command.
For operation responses:

```json
{
  "result": "PING 1.1.1.1 (1.1.1.1) ...\n...",
  "timestamp": "2026-05-14T16:21:00Z"
}
```

`result` is decoded from `bytes` to a string for ergonomics; if
you need the raw bytes, use `--output raw`.

For `routers`:

```json
{
  "routers": [
    {"id": 1, "name": "rt1.sto1", "location": "Stockholm", "healthy": true, "checked_at": "2026-05-14T16:20:00Z"}
  ],
  "next_page": 0
}
```

### `raw`

Just the result bytes. No metadata, no timestamps, no JSON. Useful
for piping into shell tools:

```bash
lg-cli -o raw bgp route as203038 1 8.8.8.0/24 | grep AS-Path
```

## Exit codes

| Code | Meaning                                                                  |
| ---- | ------------------------------------------------------------------------ |
| 0    | Success.                                                                  |
| 1    | Generic error (RPC failure, unknown subcommand, invalid args, …).         |
| 130  | Interrupted via SIGINT (`Ctrl-C`).                                        |

Errors are written to stderr with a `Error:` prefix in pretty mode.

## Signals and cancellation

`lg-cli` installs a `signal.NotifyContext` for `SIGINT` /
`SIGTERM` and cancels the request context on receipt. This means
hitting `Ctrl-C` during a slow RPC closes the connection
immediately rather than waiting for the request timeout. Cobra's
help output is unaffected.

## Examples

### Quick check of a private instance

```bash
lg-cli info http://10.0.0.5:8080
lg-cli routers http://10.0.0.5:8080
lg-cli ping http://10.0.0.5:8080 1 1.1.1.1
```

### Scripted BGP poll

```bash
#!/usr/bin/env bash
set -euo pipefail

INSTANCE="as203038"
ROUTERS="$(lg-cli -o json routers "$INSTANCE" | jq -r '.routers[].id')"
PREFIX="8.8.8.0/24"

for id in $ROUTERS; do
  echo "== router $id =="
  lg-cli -o raw --timeout 30s --quiet bgp route "$INSTANCE" "$id" "$PREFIX"
done
```

### Honouring `NO_COLOR` in CI

```bash
NO_COLOR=1 lg-cli routers as203038
```

Or per-invocation:

```bash
lg-cli --no-color routers as203038
```

### Long-running BGP query

```bash
lg-cli --timeout 5m bgp route as203038 1 0.0.0.0/0
```

The default 60s timeout is comfortable for ping/traceroute and
small lookups; full-table BGP queries can take minutes on slow
routers.

## Environment variables summary

```
LG_INDEX_URL      override --index
LG_OUTPUT         override --output (pretty|json|raw)
LG_TIMEOUT        override --timeout (Go duration)
LG_NO_COLOR       any non-empty value disables ANSI
NO_COLOR          standard https://no-color.org/ — same effect
```

There is no env var for the instance or router argument — those
are positional. If you find yourself needing one, alias the
command:

```bash
alias mylg='lg-cli https://lg.example.net'
mylg ping 1 1.1.1.1
```

## Limitations

* **Catalogue caching** — `lg-cli` does not cache the router
  catalogue locally. Every `routers` / name-resolution call hits
  the server. Use numeric IDs in tight loops if this matters.
* **No streaming output** — every command waits for the full RPC
  response before printing. There is no per-line progress for
  long traceroutes; the router buffers locally and the response
  is delivered in one go.
* **No TLS verification opt-out** — the underlying ConnectRPC
  client uses Go's default TLS config. Self-signed or expired
  certificates will fail. If you need to talk to such an
  endpoint, use the JSON-over-POST protocol with `curl -k` (see
  [api.md](./api.md)).

## Internals (for contributors)

The CLI is structured as one file per concern:

| File                     | Purpose                                                      |
| ------------------------ | ------------------------------------------------------------ |
| `cmd/cli/main.go`        | Entry point; six lines, runs `newRootCmd().Execute()`.        |
| `cmd/cli/root.go`        | Global flags, env-var fallbacks, context construction.       |
| `cmd/cli/client.go`      | ConnectRPC client factory (per-instance).                     |
| `cmd/cli/index.go`       | Public-index fetch + name/ASN/URL resolution.                 |
| `cmd/cli/output.go`      | Pretty / JSON / raw printers; ANSI handling.                  |
| `cmd/cli/cmd_instances.go` | `instances` subcommand.                                     |
| `cmd/cli/cmd_info.go`    | `info` subcommand.                                            |
| `cmd/cli/cmd_routers.go` | `routers` subcommand + router-arg resolution.                |
| `cmd/cli/cmd_pingtrace.go` | `ping` / `traceroute` subcommands.                          |
| `cmd/cli/cmd_bgp.go`     | `bgp summary` / `route` / `community` / `aspath`.            |
| `cmd/cli/cmd_meta.go`    | `version`, `completion`.                                      |

When adding a new subcommand, mirror the structure: one file
named `cmd_<verb>.go` exposing `new<Verb>Cmd() *cobra.Command`,
called from `newRootCmd()` in `root.go`.
