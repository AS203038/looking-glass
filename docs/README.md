# Looking Glass — Documentation

Looking Glass is a modern, stateless network-diagnostic platform: a single self-contained Go binary that fronts a fleet of routers over SSH and exposes ping / traceroute / BGP lookups through a gRPC (ConnectRPC) API, a CLI client (`lg-cli`), and an embedded SvelteKit web UI.

This `docs/` tree is the authoritative documentation for operators, contributors, and integrators. It is intentionally task-oriented: each file answers a specific question rather than describing the codebase in the abstract.

## Documentation map

| File                                                | Audience            | What you'll find                                                                                                  |
| --------------------------------------------------- | ------------------- | ----------------------------------------------------------------------------------------------------------------- |
| [getting-started.md](./getting-started.md)         | New operators       | Five-minute path from `git clone` to a running server with the demo config.                                       |
| [configuration.md](./configuration.md)             | Operators           | Every `config.yaml` key, defaults, environment variables, validation rules.                                       |
| [bmp.md](./bmp.md)                                 | Operators / SREs    | BGP Monitoring Protocol (BMP) guide: architecture, mutual exclusivity, benefits, drawbacks, and configuration.  |
| [deployment.md](./deployment.md)                   | Operators / SREs    | Production deployment: Docker, Kubernetes, reverse proxies, TLS, Redis, Sentry, scaling, observability.           |
| [router-templates.md](./router-templates.md)       | Template authors    | Authoring guide for vendor YAML templates: schema, variables, conventions, troubleshooting, and shipped examples. |
| [router-hardening.md](./router-hardening.md)       | Operators / Security | Per-vendor least-privilege role/class/profile snippets for the LG's SSH user (Arista, Cisco, Juniper, Nokia, MikroTik, FRR). |
| [architecture.md](./architecture.md)               | Anyone curious      | How the pieces fit together: request lifecycle, middleware chain, concurrency model, embed pipeline.              |
| [development.md](./development.md)                 | Contributors        | Local development loop, project layout, Makefile targets, codegen, testing, release process.                      |
| [api.md](./api.md)                                 | API consumers       | gRPC / ConnectRPC contract reference, error mapping, caching semantics, gRPC-Web notes.                           |
| [cli.md](./cli.md)                                 | CLI users           | `lg-cli` reference: subcommands, instance/router resolution, output modes, shell completion.                      |

## Conventions used in this documentation

* **Shell snippets** start from the repository root unless noted.
* **Configuration snippets** are valid YAML — copy-paste should work.
* **Commands** are POSIX (`bash`). Windows users should adapt paths.
* **Variables** in code references use Go's godoc form (e.g. `utils.Config`, `routers.Yaml`); browse the source at the same path under `pkg/` / `cmd/`.

## Where things live in the repository

```
.
├── cmd/                     # Binary entry points
│   ├── server/main.go       # Looking Glass HTTP/2 daemon (embeds WebUI)
│   └── cli/                 # lg-cli (cobra subcommands)
├── pkg/                     # Reusable Go packages
│   ├── errs/                # Sentinel errors surfaced to clients
│   ├── http/                # HTTP/2 listener, middleware chain
│   │   ├── grpc/            # ConnectRPC service + health checker
│   │   └── webui/           # /_app/env.js runtime-config injector
│   ├── routers/             # Template registry + bundled *.yml
│   └── utils/               # Config, SSH, IP/CIDR, sanitisers, TLS, version
├── protobuf/                # buf-managed .proto + generated Go/TS
├── webui/                   # SvelteKit 5 + Tailwind 4 + Catppuccin UI
├── example.config.yaml      # Annotated reference config
├── public_index.yaml        # Registry of public Looking Glass instances
├── Dockerfile               # Multi-stage build: buf → pnpm → go
├── Makefile                 # All local-dev tasks (`make help`)
└── docs/                    # ← you are here
```

## Quick links

* **Demo instance:** <https://lg.as203038.net/>
* **Public index:** [`public_index.yaml`](../public_index.yaml)
* **Issue tracker:** <https://github.com/AS203038/looking-glass/issues>
* **License:** GPL-3.0-or-later

If you spot a documentation gap, please open an issue or PR — the docs ship in-tree precisely so they can be fixed at the same time as the code.
