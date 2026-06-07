# Looking Glass

A modern, stateless network-diagnostic platform — a single self-contained Go binary that fronts a fleet of routers over SSH and exposes ping / traceroute / BGP lookups through a gRPC (ConnectRPC) API, an embedded SvelteKit web UI, and a `lg-cli` client.

Built to replace the abandoned/ancient looking glass projects most of us are still running.

**Demo instance:** <https://lg.as203038.net/> (used as a daily driver by AS203038).

## Why?

- **Modern stack** — Go + ConnectRPC + SvelteKit. Single binary, embedded UI, HTTP/2 + gRPC-Web out of the box.
- **Extensible** — vendor support is YAML data, not Go code. Add a new router type without recompiling. See [Router Templates](./docs/router-templates.md).
- **Production-ready** — stateless, horizontally scalable, optional Redis cache, optional Sentry integration, health checking.
- **Multiple vendors out of the box** — FRRouting, Cisco IOS/IOS-XE, Arista EOS, Juniper JunOS, Nokia SR OS, MikroTik RouterOS.
- **CLI + WebUI + native gRPC** — pick whichever surface fits.

## Quick start

```bash
# Run a released binary
curl -L -o looking-glass \
  https://github.com/AS203038/looking-glass/releases/latest/download/looking-glass-linux-amd64
chmod +x looking-glass
curl -L -o config.yaml \
  https://raw.githubusercontent.com/AS203038/looking-glass/main/example.config.yaml
$EDITOR config.yaml
./looking-glass
```

Or build from source / Docker — full walk-through in [**Getting Started**](./docs/getting-started.md).

## Documentation

All documentation lives in [`docs/`](./docs/). Start with the [index](./docs/README.md) or jump straight to:

| Topic                                                     | Use when…                                                            |
| --------------------------------------------------------- | -------------------------------------------------------------------- |
| [Getting Started](./docs/getting-started.md)              | You want a running server in five minutes.                            |
| [Configuration](./docs/configuration.md)                  | You need the authoritative `config.yaml` reference.                   |
| [Deployment](./docs/deployment.md)                        | You're shipping to production (systemd / Docker / Kubernetes).        |
| [Architecture](./docs/architecture.md)                    | You want to understand how the pieces fit together.                   |
| [Router Templates](./docs/router-templates.md)            | You need to add a vendor / write or override a router template.       |
| [API Reference](./docs/api.md)                            | You're integrating with the gRPC / ConnectRPC API.                    |
| [CLI Reference](./docs/cli.md)                            | You're using or scripting against `lg-cli`.                           |
| [Development](./docs/development.md)                      | You're contributing — local dev loop, codegen, conventions.           |

## Public index — add your instance!

Looking Glass ships with a CLI (`lg-cli`) that can address any instance by **friendly name** or **ASN** rather than a full URL:

```bash
lg-cli ping as203038 1 1.1.1.1
lg-cli routers AS203038
```

That lookup is powered by [`public_index.yaml`](./public_index.yaml) — a small, plain-text registry of public Looking Glass deployments. Right now it contains exactly one entry. **It will only become genuinely useful when more operators add theirs.**

### 📣 If you run a public Looking Glass instance — please send a PR.

It's a five-line addition:

```yaml
index:
  - name: "QuxLabs"
    asn: 203038
    url: "https://lg.as203038.net/"
  - name: "Your Network"            # ← add your block
    asn: 65000
    url: "https://lg.example.net/"
```

The instance doesn't need to run this Looking Glass — any ConnectRPC-speaking endpoint that implements the `lookingglass.v0.LookingGlassService` contract works. (Old PHP or Perl looking glasses don't qualify; this is part of the point of replacing them.) Open a PR against the file and that's it.

For details on the API contract every indexed instance is expected to honour, see [API Reference](./docs/api.md).

## Project status

Production-ready and actively maintained. The AS203038 demo instance is a daily-driver deployment.

Currently bundled router templates: FRRouting, Cisco IOS/IOS-XE, Arista EOS, Juniper JunOS, Nokia SR OS, MikroTik RouterOS.

## Contributing

Contributions are welcome — especially **new router templates**. If you can give us read-only access to a vendor we don't yet support, we'll happily write the template ourselves. File an issue.

See [Development](./docs/development.md) for the local dev loop and conventions.

## License

GPL-3.0-or-later. See [`LICENSE`](./LICENSE).
