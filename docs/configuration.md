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
| `enabled` | bool   | `false` | When false, the cache middleware is not installed.                                                                                              |
| `uri`     | string | —       | Parsed by `redis.ParseURL`. Standard `redis://` and `rediss://` schemes; `?protocol=3` enables RESP3. Parse failure disables the cache and logs. |
| `ttl`     | string | `1m`    | Go [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) string. Malformed values fall back to 60 seconds and log a warning.            |

The cache key is `md5(request_path || request_body)`. Hits replay
the entire stored response (status, headers, body) and set
`X-Cache: HIT` so the access log shows cache effectiveness:

```
192.0.2.42 "POST /lookingglass.v0.LookingGlassService/Ping HTTP/2.0" 200 1234 "" "grpc-web/1.0" 12.4ms HIT
```

There is no manual invalidation API. Wait for the TTL or flush the
Redis database.

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

Any parse or read error at this stage **panics** — a malformed
template would otherwise render the device that references it
permanently broken at request time, and failing at startup is
strictly better than failing on every request. Validate templates
before deploying them.

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
| `config.yaml` missing                   | `log.Fatalf` (process exits).                                          |
| Malformed YAML                          | `log.Fatalf` (process exits).                                          |
| Device without `hostname`               | Silently dropped from `devices`.                                       |
| Device without `source4` / `source6`    | Defaults inserted (`127.0.0.1` / `::1`).                                |
| Device with unknown `type:`             | Logged and skipped; remaining devices keep their IDs from list order. |
| Invalid `redis.ttl`                     | Falls back to 60s; logged at WARNING.                                  |
| Invalid `redis.uri`                     | Cache disabled; logged at ERROR; server keeps starting.                |
| Bundled template name clash             | Embedded copy skipped; external (ROUTER_DIR) copy wins; warning logged. |
| External template parse error           | Panic at startup.                                                      |
| Template missing requested op           | RPC returns `errs.OperationUnknown` ("operation unknown").             |
