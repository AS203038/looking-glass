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
6. Runs the vendor-template-declared parser (if any) and projects
   its outcome onto the response's `parsed` / `parser_kind` /
   `parse_status` fields. See [Structured output](#structured-output)
   below.

The `result` bytes are **always** populated, even when a parser
ran successfully — they remain the authoritative source for
copy-paste, search, and any client that does not yet understand
the structured `parsed` payload.

### `Ping`

```
Ping(PingRequest) → PingResponse
```

| Request field | Type   | Description                                                                |
| ------------- | ------ | -------------------------------------------------------------------------- |
| `router_id`   | int64  | 1-based router ID from `GetRouters`.                                       |
| `target`      | string | Destination. IPv4, IPv6, CIDR, or hostname. Hostnames are resolved to the first DNS answer. |

| Response field | Type            | Description                                                                                            |
| -------------- | --------------- | ------------------------------------------------------------------------------------------------------ |
| `result`       | bytes           | Concatenated stdout of every command in the template.                                                  |
| `timestamp`    | `Timestamp`     | When the response was assembled.                                                                       |
| `parsed`       | `PingStats`     | Optional. Populated when a parser is configured for `ping` and ran successfully. See below.            |
| `parser_kind`  | `ParserKind`    | Which parser pipe produced `parsed` (`textfsm`, `native_json`, `builtin`, or `UNSPECIFIED`).            |
| `parse_status` | `ParseStatus`   | Outcome of the parser attempt (`ok`, `disabled`, `template_missing`, `parse_failed`).                  |

### `Traceroute`

```
Traceroute(TracerouteRequest) → TracerouteResponse
```

Same request shape as `Ping`. Response carries the same five
fields as `PingResponse` except `parsed` is a `TracerouteParsed`
(see [Structured output](#structured-output)).

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

## Structured output

Every operation response carries three fields in addition to the
raw `result` bytes:

| Field          | Type            | Meaning                                                                                              |
| -------------- | --------------- | ---------------------------------------------------------------------------------------------------- |
| `parsed`       | typed message   | Optional. Structured form of `result`; one of `PingStats`, `TracerouteParsed`, `BGPSummaryParsed`, `BGPPaths`. |
| `parser_kind`  | `ParserKind`    | Names the parser pipe that produced `parsed`: `TEXTFSM`, `NATIVE_JSON`, `BUILTIN`, or `UNSPECIFIED`. |
| `parse_status` | `ParseStatus`   | Outcome of the parser attempt: `OK`, `DISABLED`, `TEMPLATE_MISSING`, `PARSE_FAILED`.                 |

### Policy: router CPU is the bottleneck

Looking Glass treats parsing as something it does on its own
machine, not something it asks the router to do for free. The
default for every vendor is **plain CLI → TextFSM on the
Looking Glass**. Vendor-native JSON pipelines
(`| display json` on JunOS, `| json` on Arista / Cisco IOS-XE)
re-serialise text the router has already rendered and typically
cost 1.5–3× the CPU and 2–4× the byte volume — they are
deliberately opt-in per template.

The only routinely-recommended `NATIVE_JSON` source today is
**FRRouting**: FRR holds the RIB as structured data in its
userland daemon, so the JSON pipe is essentially a `memcpy`.
Other vendors stay on TextFSM until benchmarks demonstrate the
JSON path is cheap for them too.

### Authoritativeness

`parsed` is **never authoritative over `result`**:

* Every WebUI/CLI/curl path keeps rendering `result` as the
  fallback when `parse_status != OK`.
* When `parse_status == OK`, the structured view is the
  primary UI but `result` is still the canonical bytes for copy/
  paste, search, and downstream tooling.
* Cached responses from older binaries (without `parsed`) remain
  fully readable by new clients — `parsed` is an additive field.

### Payload shapes

`PingStats`:

| Field              | Type    | Notes                                                            |
| ------------------ | ------- | ---------------------------------------------------------------- |
| `target`           | string  | Address actually probed (may differ from request when resolved). |
| `source`           | string  | Bound source address, when exposed.                              |
| `packets_sent`     | uint32  |                                                                  |
| `packets_received` | uint32  |                                                                  |
| `loss_pct`         | float32 | 0–100.                                                           |
| `rtt_min_ms`       | float32 | 0 when not reported.                                             |
| `rtt_avg_ms`       | float32 |                                                                  |
| `rtt_max_ms`       | float32 |                                                                  |
| `rtt_mdev_ms`      | float32 | Standard deviation; 0 when not reported.                         |

`TracerouteParsed`:

| Field    | Type                  | Notes                                            |
| -------- | --------------------- | ------------------------------------------------ |
| `target` | string                | Destination address as reported by the router.   |
| `source` | string                | Bound source address, when exposed.              |
| `hops`   | `repeated TracerouteHop` | One row per TTL.                              |

`TracerouteHop` is `{ttl, probes[]}`; `TracerouteProbe` is
`{ip, hostname, rtt_ms, asn}`. A timed-out hop carries one
synthetic empty probe (all fields zero).

`BGPSummaryParsed`:

| Field       | Type                  | Notes                                          |
| ----------- | --------------------- | ---------------------------------------------- |
| `local_asn` | uint32                | Local AS, when reported.                       |
| `router_id` | string                | BGP router-id, when reported.                  |
| `peers`     | `repeated BGPPeer`    | One row per BGP neighbour.                     |

`BGPPeer` carries `peer_ip`, `peer_asn`, `description`,
`state` (normalised to lower-case: `established`, `idle`,
`active`, `connect`, `opensent`, `openconfirm`), `state_detail`
(vendor sub-state, free-form), `uptime_seconds`,
`prefixes_received`, `prefixes_accepted`, `prefixes_sent`,
`address_family` (`ipv4-unicast` / `ipv6-unicast` / …).

`BGPPaths` (used for `bgp.route`, `bgp.community`,
`bgp.largecommunity`, `bgp.aspath`):

| Field   | Type               | Notes                                            |
| ------- | ------------------ | ------------------------------------------------ |
| `paths` | `repeated BGPPath` | One row per matched path, vendor-supplied order. |

`BGPPath` carries `prefix`, `nexthop`, `as_path` (`repeated uint32`),
`origin` (`igp`/`egp`/`incomplete`), `med`, `local_pref`,
`communities` (`repeated string` in `ASN:VALUE` form),
`large_communities` (`repeated string` in `GLOBAL:L1:L2`),
`best` (bool), `peer_asn`, `peer_ip`, `age_seconds`.

### Status decoding

| `parse_status`              | What clients should render                                                            |
| --------------------------- | ------------------------------------------------------------------------------------- |
| `PARSE_STATUS_OK`           | Structured view from `parsed`; `result` available as a toggle / fallback.             |
| `PARSE_STATUS_DISABLED`     | No parser configured for this op. Render `result` as-is.                              |
| `PARSE_STATUS_TEMPLATE_MISSING` | Packaging defect — parser was requested but its template/schema asset was absent. Surface a quiet warning ("structured view unavailable: template missing"), render `result`. |
| `PARSE_STATUS_PARSE_FAILED` | Parser ran but produced nothing useful (vendor output drift, malformed JSON, …). Quiet warning, render `result`. |

When `parse_status != OK` and `parser_kind != UNSPECIFIED`,
clients can use `parser_kind` to tell operators which pipe
*tried*: "parser=textfsm, status=parse_failed" is operator-
actionable (template needs updating) in a way that "structured
view unavailable" is not.

## Caching semantics

When the optional Redis cache is enabled (`redis.enabled: true`),
the individual gRPC handlers in `pkg/http/grpc/service.go` automatically
cache successful protobuf responses in Redis.

The cache keys are built using granular, method-specific parameters and
the current server version:

* `Ping`: `lg:rpc:<version>:ping:<router_id>:<target>`
* `Traceroute`: `lg:rpc:<version>:traceroute:<router_id>:<target>`
* `BGPSummary`: `lg:rpc:<version>:bgpsummary:<router_id>`
* `BGPRoute`: `lg:rpc:<version>:bgproute:<router_id>:<target>`
* `BGPCommunity`: `lg:rpc:<version>:bgpcommunity:<router_id>:<community>`
* `BGPLargeCommunity`: `lg:rpc:<version>:bgplargecommunity:<router_id>:<community>`
* `BGPASPath`: `lg:rpc:<version>:bgpaspath:<router_id>:<md5(pattern)>`

This method-specific key structure offers several key benefits:

* **Build-Stable Invalidation**: The current version string (e.g. `v1.2.3` or untracked suffix) is folded directly into every key prefix. Deploying a new binary release instantly invalidates prior cached items cluster-wide, eliminating stale-state issues after an upgrade.
* **No Error Caching**: Only successful RPC responses are cached. Connection failures, authentication timeouts, or other operational errors are never written to the cache.
* **TTL Configuration**: Cache TTL is configured globally via `redis.ttl` (e.g. `5m`). There is no manual invalidation API.

Cache hits set the `X-Cache: HIT` response header and appear in the structured access log:

```json
{"time":"2026-05-16T06:00:00Z","level":"INFO","msg":"http access","component":"httpaccess","remote":"192.0.2.1","method":"POST","uri":"/lookingglass.v0.LookingGlassService/Ping","status":200,"duration":12000000,"cache":"HIT"}
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

Use this for Kubernetes readiness probes — prefer the native
`grpc:` probe (k8s >=1.24, GA 1.27) over `httpGet:`, since
`grpc.health.v1.Health/Check` is a real gRPC method and is
**POST-only**. A bare `curl -X GET` returns
`405 Method Not Allowed`; the wire formats the handler accepts
(native gRPC over HTTP/2, gRPC-Web, Connect / Connect-JSON) are
all POST. For a quick curl sanity check:

```bash
curl -fsS -X POST -H 'Content-Type: application/json' -d '{}' \
     http://localhost:8080/grpc.health.v1.Health/Check
# {"status":"SERVING_STATUS_SERVING"}
```

The standard `grpc_health_probe` CLI also works against the same
endpoint.

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
