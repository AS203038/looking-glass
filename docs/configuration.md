# Configuration Reference

Looking Glass is configured by a single YAML file: `config.yaml` in
the server's working directory. There is no command-line flag for
config path, no environment-variable equivalents for individual
keys, and no runtime reload — restart the server to apply changes.

The only environment variable consulted by the server itself is
**`ROUTER_DIR`** (see [Router templates](#router-templates)).

The canonical, fully-annotated example lives at
[`example.config.yaml`](../example.config.yaml). This page is the
spec.

## Top-level shape

```yaml
devices:        []          # required (may be empty but key must exist)
grpc:           {}          # gRPC/HTTP listener
web:            {}          # embedded WebUI + runtime env
redis:          {}          # optional response cache
logging:        {}          # optional slog-based event-stream settings
security.txt:   {}          # optional RFC 9116 endpoint
```

YAML unknown keys are ignored (yaml.v2 default), so older configs
keep working after upgrades. Validation happens once at startup in
`utils.ValidateConfig`:

* Devices without a `hostname` are silently dropped.
* Devices without `source4` default to `127.0.0.1`.
* Devices without `source6` default to `::1`.

## `devices` (required)

A list of routers. Order matters: a router's stable ID is its
1-based position in this list (`devices[0]` is ID 1, etc.). The ID
is exposed to clients via gRPC, the CLI, and in WebUI URLs.

```yaml
devices:
  - name: "rt1.sto1.example"          # required, free text
    type: "frrouting"                 # required, must match a template
    location: "Stockholm, Sweden"     # optional, used to group in WebUI
    hostname: "10.0.0.1:22"           # required, host[:port]
    username: "lg"                    # required for SSH login
    password: "secret"                # optional (use ssh_key instead)
    ssh_key:  "/run/secrets/lg-key"   # optional, takes precedence if set
    source4:  "192.0.2.1"             # required for IPv4 ping/traceroute
    source6:  "2001:db8::1"           # required for IPv6 ping/traceroute
    vrf:      "default"               # required (use "default" / "main" / "Base" if you don't run VRFs)
```

### Field semantics

| Key         | Type   | Default     | Notes                                                                                                                                          |
| ----------- | ------ | ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`      | string | —           | Shown in the picker and CLI. No syntax restrictions.                                                                                           |
| `type`      | string | —           | Must equal the `name:` field of a loaded router template. Unknown types log an error and the device is skipped (server still starts).          |
| `location`  | string | `""`        | Free-form site label. Routers sharing a `location` are grouped in the WebUI picker; missing/empty becomes "Other".                             |
| `hostname`  | string | —           | SSH endpoint. `host[:port]`. No `port` defaults to 22 from `ssh.Dial`. Devices without this are dropped at validation.                          |
| `username`  | string | —           | SSH login.                                                                                                                                     |
| `password`  | string | `""`        | Always added as an SSH auth method. Ignored at handshake time when the router prefers public-key auth.                                         |
| `ssh_key`   | string | `""`        | Filesystem path to a PEM-encoded private key. Read at every connection. When set, it's appended to the auth methods.                          |
| `source4`   | IPNet  | `127.0.0.1` | IPv4 source address bound on ping/traceroute. Used as `{{.Cfg.Source4.IP}}` in templates.                                                      |
| `source6`   | IPNet  | `::1`       | IPv6 source address bound on ping/traceroute.                                                                                                  |
| `vrf`       | string | `""`        | Routing-instance name interpolated as `{{.Cfg.VRF}}`. **Every bundled template threads this through every operation.** Use the platform default (`default` for IOS/EOS/FRR, `main` for RouterOS, `Base` for SR OS, `inet` for JunOS) if you don't run VRFs. |

### A note on credentials

Credentials are stored in plain text in the config file. The
recommended deployment posture is:

* Restrict the file's permissions (`chmod 0600` and a dedicated
  user/group).
* In Kubernetes, project the file in from a `Secret` (not a
  `ConfigMap`).
* Use SSH keys (`ssh_key:`) over passwords where possible, and put
  the key in a `Secret` too.
* Bind a per-router, read-only account on the router. Looking Glass
  never issues configuration commands, but defence in depth is
  cheap.

## `grpc` (HTTP/2 listener)

```yaml
grpc:
  enabled: true
  listen: ":8080"
  tls:
    enabled: false
    self_signed: false
    cert: "/path/to/fullchain.pem"
    key:  "/path/to/privkey.pem"
```

| Key                | Type   | Default | Notes                                                                                              |
| ------------------ | ------ | ------- | -------------------------------------------------------------------------------------------------- |
| `enabled`          | bool   | `false` | When false, the gRPC service handlers are not mounted. The WebUI can still serve, but it will be useless. |
| `listen`           | string | —       | Bind address. `:8080` for "all interfaces, port 8080".                                              |
| `tls.enabled`      | bool   | `false` | When false the listener runs h2c (HTTP/2 cleartext). Required for gRPC clients that won't downgrade. |
| `tls.self_signed`  | bool   | `false` | Generate an in-memory cert at startup via `utils.GenerateSelfSignedPair`. Useful behind an ingress that re-terminates TLS. |
| `tls.cert`         | string | —       | PEM-encoded chain. Ignored when `self_signed` is true.                                              |
| `tls.key`          | string | —       | PEM-encoded private key. Ignored when `self_signed` is true.                                        |

The listener serves *all* HTTP surfaces from this one port:

* `/lookingglass.v0.LookingGlassService/*` — ConnectRPC handlers
* `/grpc.health.v1.Health/*` — gRPC health protocol
* `/_app/env.js` — WebUI runtime-config injector
* `/.well-known/security.txt` — only when `security.txt.enabled`
* `/*` — embedded SvelteKit static bundle

There is no way to disable the WebUI mount independently of the
gRPC mount; toggle `web.enabled` instead.

## `web` (embedded WebUI)

The WebUI is a SvelteKit static build embedded into the Go binary
at compile time. This section controls a small runtime
configuration object (`/_app/env.js`) the WebUI fetches on boot.

```yaml
web:
  enabled: true
  grpc_url: ""                        # empty = same origin as the page
  title: "AS203038 Looking Glass"
  header:
    text: "AS203038"
    logo: "/logo.svg"                 # optional URL
    links:
      - text: "Status"
        url: "https://status.example.net"
  footer:
    text: "© 2026 Example"
    logo: ""
    links:
      - text: "Source"
        url: "https://github.com/AS203038/looking-glass"
  sentry:
    enabled: false
    dsn: "https://...sentry.io/..."
    environment: "production"
    sample_rate: 0.1
```

| Key                  | Type    | Default | Notes                                                                                            |
| -------------------- | ------- | ------- | ------------------------------------------------------------------------------------------------ |
| `enabled`            | bool    | `false` | When false, neither `/_app/env.js` nor static files are mounted.                                  |
| `grpc_url`           | string  | `""`    | Origin the WebUI dials for gRPC-Web. Empty means "same origin as the served page". Set when the WebUI and gRPC live on different hosts. |
| `title`              | string  | `""`    | `<title>` of the page.                                                                            |
| `header.text`        | string  | `""`    | Brand bar text.                                                                                   |
| `header.logo`        | string  | `""`    | URL to a logo image; renders next to `header.text`.                                               |
| `header.links`       | list    | `[]`    | Links rendered in the header. Each entry is `{ text, url }`.                                      |
| `footer.text`        | string  | `""`    | Footer text.                                                                                      |
| `footer.logo`        | string  | `""`    | URL to a logo image; renders next to `footer.text`.                                               |
| `footer.links`       | list    | `[]`    | Links rendered in the footer.                                                                     |
| `sentry.enabled`     | bool    | `false` | Toggles **both** the server-side `sentry-go` middleware AND the WebUI's lazy `@sentry/svelte` init. |
| `sentry.dsn`         | string  | `""`    | Sentry project DSN. Forwarded to the browser when `sentry.enabled` is true.                       |
| `sentry.environment` | string  | `""`    | Tag attached to every event. Treated by Sentry as "production" when blank.                        |
| `sentry.sample_rate` | float   | `0.0`   | Traces-sample-rate `[0,1]`. `0` keeps error reporting but disables performance tracing.            |

The link list is serialised into a `"name|href,name|href"` string
in `/_app/env.js` (see `utils.HFBlock.LinksString`). This is an
implementation detail of the env bridge and not part of any public
API.

## `redis` (optional response cache)

```yaml
redis:
  enabled: true
  ttl: 5m
  uri: "redis://redis-svc.lg.svc.cluster.local:6379/0?protocol=3"
```

| Key       | Type   | Default | Notes                                                                                                                                          |
| --------- | ------ | ------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled` | bool   | `false` | When false, the shared response cache is disabled.                                                                                              |
| `uri`     | string | —       | Parsed by `redis.ParseURL`. Standard `redis://` and `rediss://` schemes; `?protocol=3` enables RESP3. Parse failure disables the cache and logs. |
| `ttl`     | string | `1m`    | Go [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) string. Malformed values fall back to 60 seconds and log a warning.            |

RPC payloads are cached inside the ConnectRPC layer (under `pkg/http/grpc/`). 
The cache keys are method-specific and structured as follows:

* `Ping`: `lg:rpc:<version>:ping:<router_id>:<target>`
* `Traceroute`: `lg:rpc:<version>:traceroute:<router_id>:<target>`
* `BGPSummary`: `lg:rpc:<version>:bgpsummary:<router_id>`
* `BGPRoute`: `lg:rpc:<version>:bgproute:<router_id>:<target>`
* `BGPCommunity`: `lg:rpc:<version>:bgpcommunity:<router_id>:<community>`
* `BGPLargeCommunity`: `lg:rpc:<version>:bgplargecommunity:<router_id>:<community>`
* `BGPASPath`: `lg:rpc:<version>:bgpaspath:<router_id>:<md5(pattern)>`

Hits serve the cached protobuf message and set the `X-Cache: HIT` 
response header so the structured access log shows cache effectiveness:

```json
{"time":"2026-05-16T06:00:00Z","level":"INFO","msg":"http access","component":"httpaccess","remote":"192.0.2.42","method":"POST","uri":"/lookingglass.v0.LookingGlassService/Ping","status":200,"duration":12400000,"cache":"HIT"}
```

There is no manual invalidation API. Wait for the TTL or flush the
Redis database.

## `logging` (optional)

The server emits every diagnostic event through `log/slog`. Records
are filtered by a configurable minimum level (default `info`),
encoded in a configurable format (default `json`), and written to a
configurable sink (default `stdout`). Each event carries a
`component=<name>` attribute identifying the subsystem so operators
can grep / filter / aggregate by subsystem.

```yaml
logging:
  level: info               # debug | info | warn | error
  format: json              # json | text
  output: stdout            # stdout | stderr | <file path>
  source: false             # add file:line annotations
  components:               # per-component level overrides
    ssh: debug
    parse: warn
```

| Key          | Type   | Default  | Notes                                                                                                                                          |
| ------------ | ------ | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `level`      | string | `info`   | Default minimum level for all components.                                                                                                       |
| `format`     | string | `json`   | `json` for machine-parseable output; `text` for slog's `key=value` form when tailing a terminal.                                               |
| `output`     | string | `stdout` | Sink. `stdout`, `stderr`, or a filesystem path. Files are opened with `O_APPEND|O_CREATE`; rotate them out-of-process.                          |
| `source`     | bool   | `false`  | When true, each record carries `source` (file:line). Useful for debugging at higher cost.                                                       |
| `components` | map    | `{}`     | Per-component level overrides; a component's threshold replaces `level` for that subsystem. Unknown components are accepted (no-op).            |

The HTTP access log, previously emitted in Apache Common Log Format,
is now a structured `INFO` record on the `httpaccess` component
carrying every field the old single-line format carried.

### Components

Each component emits events at the levels below. Setting `level: debug`
globally (or for one component via `components: { name: debug }`) turns
on the additional per-request / per-tick / per-render visibility marked
"DEBUG". The default `level: info` is appropriate for production.

| Component    | DEBUG events                                                                                  | INFO/WARN/ERROR events                                                       |
| ------------ | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `server`     | —                                                                                             | `server starting` (version, pid, devices) / `server shutting down`.          |
| `http`       | —                                                                                             | `listening` / `connecting to redis` / TLS choice / Sentry init.              |
| `httpaccess` | —                                                                                             | One `INFO http access` record per request.                                   |
| `cache`      | `cache hit` / `cache miss` / `cache entry malformed` / `cache store ok`.                      | `marshal cache entry failed` / `store cache entry failed` (ERROR).           |
| `rpc`        | `rpc start` (per handler entry).                                                              | `rpc ok` (INFO per success: op, router, duration, parser kind/status); `rpc failed` (WARN). |
| `health`     | `healthcheck tick` (per minute) / `healthcheck probe ok` / `healthcheck probe failed` (per router). | State transitions only: `router healthy` (INFO) / `router unhealthy` (WARN). |
| `ssh`        | `ssh dial start` / `ssh dial ok` / `ssh exec start` / `ssh exec ok` / `ssh client close`.    | `dial failed` / `new session failed` / `command failed` (ERROR).             |
| `routers`    | `router bound` per device.                                                                    | `router catalogue built` summary (INFO); `router type not found` (ERROR).    |
| `yaml`       | —                                                                                             | `router registered` per template; `unknown parser kind` (WARN); load failures (ERROR + exit). |
| `tpl`        | `tpl render` per rendered command (with the final rendered string).                          | `operation not defined for router` (WARN); `parse failed` / `execute failed` (ERROR). |
| `parse`      | `parser run` / `parser ok` (per parser invocation: parser, op, raw bytes, records/paths/peers/hops). | `template missing` / `no projection` (WARN); load / exec failures (ERROR).  |
| `stdlog`     | —                                                                                             | Anything routed via the stdlib `log` package (e.g. `net/http` server errors). |

### Process exit codes

The server uses distinct exit codes for distinct startup failures so
the cause is recoverable from the shell without scraping log output:

| Code | Source                              | Meaning                                                |
| ---: | ----------------------------------- | ------------------------------------------------------ |
|    2 | `cmd/server/main.go`                | Embedded WebUI filesystem unavailable.                 |
|    3 | `cmd/server/main.go`                | `config.yaml` parse failed.                            |
|    4 | `cmd/server/main.go`                | `logging.Init` failed (bad level / format / output).   |
|   10 | `pkg/routers/routers.go::register`  | Router template registered with an empty name.         |
|   11 | `pkg/routers/routers.go::register`  | Duplicate router-template name.                        |
|   20 | `pkg/routers/yaml.go::init`         | `ROUTER_DIR` directory unreadable.                     |
|   21 | `pkg/routers/yaml.go::init`         | YAML file in `ROUTER_DIR` unreadable.                  |
|   22 | `pkg/routers/yaml.go::init`         | YAML file in `ROUTER_DIR` unparseable.                 |
|   23 | `pkg/routers/yaml.go::init`         | Embedded template directory unreadable.                |
|   24 | `pkg/routers/yaml.go::init`         | Embedded template file unreadable.                     |
|   25 | `pkg/routers/yaml.go::init`         | Embedded template file unparseable.                    |

## `security.txt` (optional)

When enabled, the server publishes an RFC 9116 document at
`/.well-known/security.txt`.

```yaml
security.txt:
  enabled: true
  contact: "mailto:security@example.net"
  canonical: "https://lg.example.net/.well-known/security.txt"
  encryption: "https://example.net/pgp.asc"
  acknowledgements: "https://example.net/security/hall-of-fame"
  preferred-languages: "en, sv"
  policy: "https://example.net/security-policy"
  hiring: "https://example.net/jobs"
  csaf: "https://example.net/.well-known/csaf/provider-metadata.json"
  expires: "2026-12-31T23:59:59Z"     # optional; auto-generates +1 year if omitted
```

All fields are rendered verbatim, one per line, in the order shown
in `utils.SecurityTxtConfig.String`. Empty fields still emit blank
values to keep the document shape deterministic.

## Router templates

In addition to the bundled `pkg/routers/*.yml` templates compiled
into the binary, the server can load **additional** templates from
a directory specified by the `ROUTER_DIR` environment variable.

```bash
ROUTER_DIR=/etc/looking-glass/routers ./looking-glass
```

Loading rules (see `pkg/routers/yaml.go`):

1. If `ROUTER_DIR` is set, every `*.yml` / `*.yaml` file in that
   directory is parsed and registered first. A template here with
   the same `name:` as a bundled template **wins** — this is the
   supported escape hatch for overriding shipped templates.
2. Bundled templates are then iterated. Each is registered only if
   no template under its name has been registered yet; otherwise a
   warning is logged and the bundled copy is skipped.

Any parse or read error at this stage logs an `ERROR` event and
exits the process with a distinct non-zero exit code (20–25,
[see exit-code table](#process-exit-codes)) — a malformed template
would otherwise render the device that references it permanently
broken at request time, and failing at startup is strictly better
than failing on every request. Validate templates before deploying
them.

See [router-templates.md](./router-templates.md) for authoring
guidance.

## A complete reference example

Trimmed copy of `example.config.yaml`:

```yaml
devices:
  - name: "rt1.sto1.example"
    type: "frrouting"
    location: "Stockholm, Sweden"
    hostname: "10.0.0.1:22"
    username: "lg"
    ssh_key: "/run/secrets/lg-key"
    source4: "192.0.2.1"
    source6: "2001:db8::1"
    vrf: "default"

grpc:
  enabled: true
  listen: ":8080"
  tls:
    enabled: false

redis:
  enabled: true
  ttl: 5m
  uri: "redis://redis-svc:6379/0?protocol=3"

web:
  enabled: true
  title: "AS65000 Looking Glass"
  header:
    text: "AS65000"
    links:
      - { text: "Home", url: "https://example.net" }
  footer:
    text: "© 2026 Example AB"
    links:
      - { text: "Source", url: "https://github.com/AS203038/looking-glass" }

security.txt:
  enabled: true
  contact: "mailto:security@example.net"
  canonical: "https://lg.example.net/.well-known/security.txt"
```

## What is *not* configurable

In case it saves you time grepping:

* SSH **host-key verification** is currently `InsecureIgnoreHostKey`.
  There is no `known_hosts` mode yet.
* SSH **connections are not pooled**: every RPC opens a new TCP
  + SSH session and tears it down. Caching mitigates the cost; if
  this becomes a real problem in your deployment, open an issue.
* The **health check interval** is hardcoded to 60 seconds (see
  `pkg/http/grpc/grpc.go:healthcheck`).
* The **WebUI router-list refresh** is hardcoded to 5 minutes (see
  `webui/src/lib/stores/routers.ts`).
* Build-time **`-ldflags "-X .../utils.release=…"`** is the only way
  to set the version string shown in `GetInfo` / WebUI footer / ETag
  / Sentry release. The Makefile and Dockerfile both handle this
  automatically.

## Validation behaviour cheat sheet

| Input                                   | Behaviour                                                              |
| --------------------------------------- | ---------------------------------------------------------------------- |
| `config.yaml` missing                   | Logged at ERROR; process exits with code 3.                            |
| Malformed YAML                          | Logged at ERROR; process exits with code 3.                            |
| Invalid `logging.*` key                 | Logged at ERROR; process exits with code 4.                            |
| Device without `hostname`               | Silently dropped from `devices`.                                       |
| Device without `source4` / `source6`    | Defaults inserted (`127.0.0.1` / `::1`).                                |
| Device with unknown `type:`             | Logged and skipped; remaining devices keep their IDs from list order. |
| Invalid `redis.ttl`                     | Falls back to 60s; logged at WARNING.                                  |
| Invalid `redis.uri`                     | Cache disabled; logged at ERROR; server keeps starting.                |
| Bundled template name clash             | Embedded copy skipped; external (ROUTER_DIR) copy wins; warning logged. |
| External template parse error           | Logged at ERROR; process exits with code 22.                           |
| Template missing requested op           | RPC returns `errs.OperationUnknown` ("operation unknown").             |
