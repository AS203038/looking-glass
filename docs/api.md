# API Reference

Looking Glass exposes a single ConnectRPC service,
`lookingglass.v0.LookingGlassService`, on the same HTTP/2 listener
that serves the WebUI. The contract is defined in
[`protobuf/lookingglass/v0/lookingglass.proto`](../protobuf/lookingglass/v0/lookingglass.proto) —
which is the single source of truth and propagates comments to
both godoc and TSDoc via `buf generate`. This page is the
operator-friendly summary.

## Protocols

ConnectRPC implements three wire protocols on the same handler.
Every method below is reachable through all three; pick whichever
your client supports.

| Protocol                  | `Content-Type`              | Best for                                   |
| ------------------------- | --------------------------- | ------------------------------------------ |
| gRPC                      | `application/grpc`          | Native gRPC clients (Go, Python, Java, …)  |
| gRPC-Web                  | `application/grpc-web`      | Browsers (`@connectrpc/connect-web`)       |
| Connect (JSON over POST)  | `application/json`          | `curl`, scripts, anything HTTP/1.1         |

The server speaks HTTP/2 (h2c or TLS) for the first two and falls
back to HTTP/1.1 + JSON for the third. The bundled CLI (`lg-cli`)
uses the gRPC protocol over HTTP/2.

## Endpoint shape

```
{base-url}/lookingglass.v0.LookingGlassService/{Method}
```

For example, against a local server:

```
http://localhost:8080/lookingglass.v0.LookingGlassService/Ping
```

Every method is unary (one request, one response) and uses POST.
There are no streaming RPCs in v0.

## Authentication

There is none. Looking Glass deliberately ships without auth — see
[deployment.md § Reverse proxy](./deployment.md#reverse-proxy) for
how to layer it on (basic auth, OIDC, IP allowlist, etc.).

## Catalogue methods

These are cheap, side-effect-free reads of in-memory state. They
do not touch any router and are safe to call frequently.

### `GetInfo`

```
GetInfo(google.protobuf.Empty) → GetInfoResponse
```

Returns the server identity:

| Field      | Type   | Description                                         |
| ---------- | ------ | --------------------------------------------------- |
| `hostname` | string | `os.Hostname()`, or `"unknown"` if the OS call fails. |
| `version`  | string | `<release>+<unix-nanos-hex>`. See `utils.Version()`. |

Used by the WebUI footer and the `lg-cli info` subcommand as a
connectivity probe.

JSON example:

```bash
curl -s -X POST \
  -H 'Content-Type: application/json' \
  --data '{}' \
  http://localhost:8080/lookingglass.v0.LookingGlassService/GetInfo
```

```json
{"hostname":"lg-1","version":"v1.0.0+18a5b6dabf45000"}
```

### `GetRouters`

```
GetRouters(GetRoutersRequest) → GetRoutersResponse
```

Returns one page of the router catalogue.

Request:

| Field        | Type     | Default | Description                              |
| ------------ | -------- | ------- | ---------------------------------------- |
| `limit`      | uint32   | `10`    | Maximum rows on this page.               |
| `page_token` | uint32   | `1`     | 1-based page index. Zero is treated as 1. |

Response:

| Field        | Type            | Description                                                         |
| ------------ | --------------- | ------------------------------------------------------------------- |
| `routers`    | `repeated Router` | One row per device, in stable catalogue order.                    |
| `next_page`  | uint32          | Token for the next page, or `0` when there are no more pages.       |
| `timestamp`  | `Timestamp`     | Server time when the response was assembled.                        |

`Router`:

| Field      | Type           | Description                                          |
| ---------- | -------------- | ---------------------------------------------------- |
| `id`       | int64          | Stable 1-based identifier (the index in `devices[]`). |
| `name`     | string         | `devices[].name` from `config.yaml`.                  |
| `location` | string         | `devices[].location`.                                 |
| `health`   | `RouterHealth` | Latest probe outcome.                                 |

`RouterHealth`:

| Field       | Type        | Description                                                    |
| ----------- | ----------- | -------------------------------------------------------------- |
| `timestamp` | `Timestamp` | When the probe completed.                                      |
| `healthy`   | bool        | `true` when the last probe completed without an SSH error.    |

The catalogue is read from in-memory state — it never touches a
router. Health snapshots are updated by the background ticker
(every 60s); this method does not trigger a probe.

## Operation methods

Each of these:

1. Looks up the router by `router_id`. Returns
   `errs.UnknownRouter` ("router unknown") on miss.
2. Parses or sanitises the operand. Returns the appropriate
   sentinel on failure.
3. Renders the vendor template (`pkg/routers/Yaml.Foo`).
4. Executes the rendered commands over SSH (`utils.SSHExec`).
5. Returns the joined stdout in `result` plus a server `timestamp`.

The result is always raw, vendor-formatted text. There is no
parsing or normalisation; the WebUI / `lg-cli` treat the bytes as
opaque output and just display them.

### `Ping`

```
Ping(PingRequest) → PingResponse
```

| Request field | Type   | Description                                                                |
| ------------- | ------ | -------------------------------------------------------------------------- |
| `router_id`   | int64  | 1-based router ID from `GetRouters`.                                       |
| `target`      | string | Destination. IPv4, IPv6, CIDR, or hostname. Hostnames are resolved to the first DNS answer. |

| Response field | Type        | Description                                              |
| -------------- | ----------- | -------------------------------------------------------- |
| `result`       | bytes       | Concatenated stdout of every command in the template.    |
| `timestamp`    | `Timestamp` | When the response was assembled.                         |

### `Traceroute`

```
Traceroute(TracerouteRequest) → TracerouteResponse
```

Same shape and semantics as `Ping`.

### `BGPSummary`

```
BGPSummary(BGPSummaryRequest) → BGPSummaryResponse
```

Returns the router's BGP-neighbour summary table. The request
takes only `router_id`. Note: not every shipped template defines
a `bgp.summary` operation — when missing, the server returns
`OperationUnknown`.

### `BGPRoute`

```
BGPRoute(BGPRouteRequest) → BGPRouteResponse
```

Looks up the best (and, where supported, alternate) BGP paths for
a prefix or address. Same request shape as `Ping` / `Traceroute`.

The bundled templates typically emit *one* command here, picking
the right family via `{{.IP.Family}}` — the operand carries the
family, so dual-family fan-out would be wasteful.

### `BGPCommunity`

```
BGPCommunity(BGPCommunityRequest) → BGPCommunityResponse
```

Lists every route tagged with the supplied RFC 1997 standard
community.

| Request field | Type           | Description                              |
| ------------- | -------------- | ---------------------------------------- |
| `router_id`   | int64          | 1-based router ID.                       |
| `community`   | `BGPCommunity` | `{asn, value}` — both int32.             |

The server reassembles `community` to the canonical
`"ASN:VALUE"` form before rendering the template.

A `nil` `community` field returns `OperationUnknown`.

### `BGPLargeCommunity`

```
BGPLargeCommunity(BGPLargeCommunityRequest) → BGPLargeCommunityResponse
```

Same shape, with an RFC 8092 large community:

| Request field | Type                 | Description                                          |
| ------------- | -------------------- | ---------------------------------------------------- |
| `router_id`   | int64                | 1-based router ID.                                   |
| `community`   | `BGPLargeCommunity`  | `{global_admin, local_data1, local_data2}` — uint32. |

The server reassembles to `"GLOBAL:LOCAL1:LOCAL2"`.

### `BGPASPath`

```
BGPASPath(BGPASPathRequest) → BGPASPathResponse
```

Lists every route whose AS-path matches the supplied regex.

| Request field | Type   | Description                                                                                            |
| ------------- | ------ | ------------------------------------------------------------------------------------------------------ |
| `router_id`   | int64  | 1-based router ID.                                                                                     |
| `pattern`     | string | AS-path regex. Server-side `SanitizeASPathRegex` allow-lists digits, `_`, optional `$`; ≤30 chars.     |

The sanitiser also prepends a leading `_` and appends a trailing
`_`/`$` if absent, so `203038` becomes `_203038$` before hitting
the router. Patterns failing the allow-list return
`ASPathMalformed`; empty patterns return `ASPathEmpty`; >30 chars
returns `ASPathTooLong`.

## Error mapping

Every server-side error path returns one of the sentinels in
`pkg/errs`. They map to ConnectRPC's underlying codes through
`connect.Error` wrapping in the standard way; in JSON they appear
as:

```json
{"code":"internal","message":"router unknown"}
```

The complete list:

| Sentinel               | Source                              | Cause                                                              |
| ---------------------- | ----------------------------------- | ------------------------------------------------------------------ |
| `UnknownRouter`        | RouterMap lookup                    | `router_id` not in `[1, len(devices)]`.                            |
| `RouterUnavailable`    | (reserved; not currently returned)  | Future use for explicit "router unhealthy" responses.              |
| `OperationUnknown`     | Template render                     | Template doesn't define the operation, or a parse/execute error.   |
| `IPInvalid`            | IP parsing                          | Target is empty or not a valid IP/CIDR; DNS lookup returned nothing.|
| `NetInvalid`           | CIDR parsing                        | Target is not a valid CIDR.                                        |
| `FamilyInvalid`        | (reserved)                          | Reserved for explicit family mismatches.                           |
| `ASPathMalformed`      | `SanitizeASPathRegex`               | Pattern contains characters outside the allow-list.                |
| `ASPathEmpty`          | `SanitizeASPathRegex`               | Pattern is empty.                                                  |
| `ASPathTooLong`        | `SanitizeASPathRegex`               | Pattern is longer than 30 characters.                              |
| `ConnectionFailed`     | `ssh.Dial`                          | TCP or SSH handshake failed before authentication.                 |
| `AuthFailed`           | SSH auth                            | SSH credentials rejected, or `ssh_key` unreadable / unparseable.   |
| `ExecFailed`           | SSH session execution               | Session creation, command exit, or stderr capture failed.          |

The intentional choice is **coarse client errors, detailed server
logs**. RPC consumers never see hostnames, command text, stderr,
or auth-error messages — those go only to the server's log
stream, tagged with `op=` / `router=` / `stage=` for greppability.

## Caching semantics

When the optional Redis cache is enabled (`redis.enabled: true`),
the HTTP middleware caches responses keyed by
`md5(request_path || raw_request_body)`. This means:

* Two requests with the same path and body get the same cached
  response, regardless of protocol (gRPC, gRPC-Web, JSON all hash
  to the same bytes by their wire-format body).
* Errors **are not cached** — the cache middleware stores whatever
  the handler wrote, so a 4xx/5xx response would in principle be
  cached, but ConnectRPC's error responses include the current
  timestamp making practical replays harmless. Don't rely on this.
* TTL is configured globally via `redis.ttl`. There is no
  per-method TTL and no manual invalidation API.

Cache hits set the `X-Cache: HIT` response header and appear in
the access log:

```
192.0.2.1 "POST /lookingglass.v0.LookingGlassService/Ping HTTP/2.0" 200 1234 "" "grpc-web/1.0" 12ms HIT
```

The catalogue methods (`GetInfo`, `GetRouters`) are also cached;
that's usually fine, but if your router catalogue changes
mid-deployment you may see stale rows for up to `TTL`.

## ETag (browser cache)

The `loggingHandler` middleware sets the response `ETag` header to
the value of `utils.Version()` whenever a release is configured
(i.e. not `"untracked"`). Browsers that re-validate with
`If-None-Match: <version>` get a 304 without the handler being
invoked. This dramatically reduces the cost of repeat page loads
for the WebUI static assets.

The `version` includes a nanosecond timestamp suffix, so every
server restart invalidates the cache — the goal is correctness on
upgrade, not maximum cacheability.

## gRPC health protocol

The server also mounts `grpc.health.v1.Health` from
`connectrpc.com/grpchealth`:

* `Check { service: "" }` — overall server health (`SERVING` after
  startup).
* `Check { service: "lookingglass.v0.LookingGlassService" }` —
  same.
* `Check { service: "lookingglass.v0.LookingGlassService/<name>" }`
  — per-router health. Updated by the background ticker; the
  status mirrors `RouterHealth.healthy` from `GetRouters`.

Use this for Kubernetes readiness probes (HTTP/2 needed) and the
standard `grpc_health_probe` tool.

## gRPC-Web specifics

For browser clients via `@connectrpc/connect-web`:

* The server is reachable directly without a gRPC-Web envoy
  proxy — ConnectRPC speaks gRPC-Web natively.
* All necessary CORS headers are in the allow-list (see
  `pkg/http/http.go` `corsHandler`):
  `Connect-Protocol-Version`, `Connect-Timeout-Ms`,
  `Grpc-Timeout`, `X-Grpc-Web`, `X-User-Agent`, …
* `sentry-trace` and `baggage` are also allow-listed, so
  distributed traces propagate end-to-end when both server and
  WebUI have Sentry enabled.

## Examples

### Go (Connect-Go, native gRPC)

```go
import (
    "context"
    "net/http"

    "connectrpc.com/connect"
    pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
    lgcon "github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
)

func main() {
    cli := lgcon.NewLookingGlassServiceClient(
        http.DefaultClient,
        "http://lg.example.net",
    )
    resp, err := cli.Ping(context.Background(), connect.NewRequest(&pb.PingRequest{
        RouterId: 1,
        Target:   "1.1.1.1",
    }))
    if err != nil {
        panic(err)
    }
    println(string(resp.Msg.GetResult()))
}
```

### TypeScript (Connect-Web from a browser)

```ts
import { createClient } from "@connectrpc/connect";
import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { LookingGlassService } from "@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb";

const client = createClient(
    LookingGlassService,
    createGrpcWebTransport({ baseUrl: "https://lg.example.net" }),
);

const resp = await client.ping({ routerId: 1n, target: "1.1.1.1" });
console.log(new TextDecoder().decode(resp.result));
```

### curl (Connect JSON-over-POST)

```bash
curl -s -X POST \
  -H 'Content-Type: application/json' \
  --data '{"router_id":1,"target":"1.1.1.1"}' \
  https://lg.example.net/lookingglass.v0.LookingGlassService/Ping
```

The JSON envelope is automatically produced and consumed by
ConnectRPC — `result` is base64-encoded bytes in the JSON form:

```json
{
  "result": "UElORyAxLjEuMS4xICgxLjEuMS4xKSAuLi4=",
  "timestamp": "2026-05-14T16:21:00Z"
}
```

Decode `result` with `base64 -d` to get the original router
output.

## Versioning policy

* The service stays at `lookingglass.v0` until a breaking change
  is genuinely required.
* New fields are added with new tag numbers; removals never
  happen on `v0`.
* New methods can appear at any time and are advertised in the
  `.proto`. Clients should not crash on unknown methods (they
  simply won't be called).
* Anything reading the response should treat `result` as opaque
  bytes — vendor output is *not* a stable contract.

When the project moves to `v1`, the `v0` service will be retained
for at least one release cycle and the migration documented.

## Where to read further

* `protobuf/lookingglass/v0/lookingglass.proto` — the spec.
* `pkg/http/grpc/service.go` — the handlers (~330 LoC, very
  readable).
* `pkg/errs/*.go` — the error sentinels (one-line definitions).
* [architecture.md](./architecture.md) — request lifecycle.
