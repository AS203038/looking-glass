# Getting Started

This guide walks you from a fresh checkout to a running Looking
Glass server in under five minutes. It assumes you already have at
least one router you can reach over SSH; if not, you can still
follow along — the server will start, expose the WebUI, and surface
the configured-but-unreachable routers as "unhealthy" in the UI.

## Prerequisites

| Tool       | Why                                       | Minimum version    |
| ---------- | ----------------------------------------- | ------------------ |
| Go         | Server + CLI build                        | 1.25.0+            |
| Node.js    | WebUI build (SvelteKit)                   | 20.x (LTS)         |
| pnpm       | WebUI package manager (workspace-linked)  | 9.x                |
| `buf`      | Protobuf codegen (only if you regenerate) | latest             |
| Docker     | Optional: containerised builds & runs     | any recent version |
| `make`     | Convenience targets                       | any                |

You only strictly need Go + Node + pnpm for a local source build.
Released binaries on GitHub bundle the WebUI already, so end users
can skip the Node toolchain entirely.

## Option A — Run a released binary

The fastest path is to grab a published release.

```bash
# Pick the latest release from
#   https://github.com/AS203038/looking-glass/releases
curl -L -o looking-glass \
  https://github.com/AS203038/looking-glass/releases/latest/download/looking-glass-linux-amd64
chmod +x looking-glass

# Drop the example config next to the binary and edit it
curl -L -o config.yaml \
  https://raw.githubusercontent.com/AS203038/looking-glass/main/example.config.yaml
$EDITOR config.yaml

# Run
./looking-glass
```

The server reads `config.yaml` from its **working directory**, not
from a path argument — so always start it from a directory that
contains `config.yaml`.

Open <http://localhost:8080/> in a browser. You should see the
WebUI; if the configured router(s) are reachable, the picker will
show green health badges within ~60 seconds.

## Option B — Build from source

```bash
git clone https://github.com/AS203038/looking-glass.git
cd looking-glass

# 1. Install all dependencies (Go modules, pnpm workspace, protobuf npm package)
make install

# 2. Build the full production artifact (WebUI + server + CLI)
make build

# This writes:
#   ./looking-glass   – the server binary (WebUI embedded)
#   ./lg-cli          – the CLI client
```

You can also build pieces individually:

```bash
make build-webui      # SvelteKit static build → cmd/server/dist
make build-server     # Go binary (requires WebUI build)
make build-cli        # lg-cli only
```

Run `make help` for the full target listing.

## Option C — Docker

A multi-stage Dockerfile is included that builds the protobuf,
WebUI, and Go binary in isolated stages, then ships a `scratch`
image with just the static binary and CA bundle.

```bash
make docker-build              # builds looking-glass:<git-describe> + :latest
make docker-run                # runs it with ./config.yaml mounted read-only

# Or run by hand:
docker run --rm -it \
  -p 8080:8080 \
  -v "$PWD/config.yaml:/config.yaml:ro" \
  ghcr.io/as203038/looking-glass:latest
```

The container has no shell and runs the binary as PID 1. See
[deployment.md](./deployment.md) for the production recipe.

## Minimum viable `config.yaml`

The shortest config that produces a useful server has exactly one
device and enables the gRPC listener:

```yaml
devices:
  - name: "rt1.example.net"
    type: "frrouting"
    location: "Stockholm, SE"
    hostname: "rt1.example.net:22"
    username: "lg"
    password: "hunter2"        # or use ssh_key: /path/to/id_ed25519
    source4: "192.0.2.1"
    source6: "2001:db8::1"
    vrf: "default"

grpc:
  enabled: true
  listen: ":8080"
  tls:
    enabled: false

web:
  enabled: true
  title: "Example LG"
```

That's it. Every other section in `example.config.yaml`
(`redis`, `security.txt`, `web.sentry`, …) is optional. Full key
reference in [configuration.md](./configuration.md).

### Pick the right `type:`

`type:` must match the `name:` field of a registered router
template. The bundled templates are:

| Template name        | File                              | Vendor / platform                        |
| -------------------- | --------------------------------- | ---------------------------------------- |
| `frrouting`          | `pkg/routers/frrouting.yml`       | FRRouting on Linux (vtysh + iputils)     |
| `cisco_ios`          | `pkg/routers/cisco_ios.yml`       | Cisco IOS / IOS-XE                       |
| `arista_eos`         | `pkg/routers/arista_eos.yml`      | Arista EOS                               |
| `juniper_junos`      | `pkg/routers/juniper_junos.yml`   | Juniper JunOS (MX / PTX / ACX / SRX)     |
| `nokia_sros`         | `pkg/routers/nokia_sros.yml`      | Nokia SR OS / TiMOS (classic CLI)        |
| `mikrotik_routeros`  | `pkg/routers/mikrotik_routeros.yml` | MikroTik RouterOS 7.x                  |

If your vendor isn't covered, you can write a YAML template without
touching Go — see [router-templates.md](./router-templates.md).

### Harden the router-side SSH user

The bundled templates assume the LG's SSH user can run `ping`,
`traceroute`, and a small set of `show bgp …` commands — *and
nothing else*. Each vendor exposes this through a different
mechanism (Arista `role`, Cisco `parser view` or privilege levels,
JunOS `login class`, SR OS `profile`, RouterOS `/user group`, FRR
`ForceCommand` wrapper). Copy-pasteable snippets for every vendor
live in [router-hardening.md](./router-hardening.md).

> **Arista EOS specifically** also needs
> `aaa authorization exec default local` somewhere in its running
> config, or the `privilege 15` keyword on `username` is silently
> ignored and you get `% Invalid input (privileged mode required)`
> for ping / traceroute. See the Arista section of
> [router-hardening.md](./router-hardening.md#arista-eos) for the
> full configuration.

Production deployments should *always* use a bespoke role; never
point the LG at an unconstrained `network-admin` / `privilege 15`
account. The role file is the security boundary between an
Internet-reachable WebUI and your fleet.

## Verifying it works

### From the WebUI

1. Open <http://localhost:8080/>.
2. Wait ~60 seconds for the first health check to complete.
3. The router picker should show your routers with green badges.
4. Pick a router, choose `ping`, type `1.1.1.1`, hit **Execute**.
5. The results sheet at the bottom should show the router's ping
   output.

### From the CLI

```bash
# Probe the server itself
./lg-cli --help

# Hit it directly (use http://… for a non-TLS local instance)
./lg-cli info http://localhost:8080
./lg-cli routers http://localhost:8080
./lg-cli ping http://localhost:8080 rt1 1.1.1.1
```

`lg-cli` resolves the first positional argument either as an entry
in the public index (e.g. `as203038`), an ASN
(`AS203038` / `203038`), or a full URL. See [cli.md](./cli.md) for
the full reference.

### Through gRPC directly

ConnectRPC accepts gRPC, gRPC-Web, and Connect's own JSON-over-HTTP
protocol on the same endpoint. The simplest sanity check is:

```bash
curl -s -X POST \
  -H 'Content-Type: application/json' \
  --data '{}' \
  http://localhost:8080/lookingglass.v0.LookingGlassService/GetInfo
# → {"hostname":"…","version":"untracked+…"}
```

## Common first-run issues

* **Browser shows "ERR_HTTP2_PROTOCOL_ERROR"** — you have TLS
  enabled but the certificate is invalid or self-signed without
  browser trust. Disable TLS for local testing or set
  `tls.self_signed: true` and accept the cert.
* **All routers stay red** — the first health probe runs after the
  60-second ticker fires. Check server logs for `SSH: dial failed`
  or `auth failed` lines; the operator log is detailed even though
  the RPC client only sees `connection error` / `authentication
  error`.
* **`failed to parse config: ...`** — `config.yaml` is missing from
  the CWD. The path is currently hardcoded; start the server from
  the directory holding the file.
* **gRPC-Web from the WebUI fails with `connection refused`** — the
  WebUI calls back to the same origin by default. If you're running
  the WebUI through `vite dev` on port 5173 against a backend on
  8080, set `web.grpc_url: "http://localhost:8080"` in
  `config.yaml`.

## Next steps

* Tune your config: [configuration.md](./configuration.md)
* Deploy to production: [deployment.md](./deployment.md)
* Add a new vendor: [router-templates.md](./router-templates.md)
* Understand how it all hangs together: [architecture.md](./architecture.md)
