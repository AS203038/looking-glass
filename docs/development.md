# Development

Practical guide for contributing to Looking Glass: setting up a
local environment, the build/test loop, project layout
conventions, and the release process.

For an architectural overview, read
[architecture.md](./architecture.md) first.

## Prerequisites

| Tool        | Purpose                          | Version            |
| ----------- | -------------------------------- | ------------------ |
| Go          | Backend                          | 1.25+              |
| Node.js     | WebUI build / dev server         | 20.x LTS           |
| pnpm        | WebUI package manager (workspace) | 9.x                |
| `buf`       | Protobuf codegen                  | latest             |
| Docker      | (optional) container builds       | any recent version |
| `make`      | Local tasks                       | any                |
| `git`       | Source control                   | any                |

Install Go and Node from your distro / nvm. For everything else:

```bash
# pnpm via corepack (ships with Node ≥ 16)
corepack enable
corepack prepare pnpm@9 --activate

# buf (optional, only needed when regenerating protobuf)
make install-tools     # equivalent to: go install github.com/bufbuild/buf/cmd/buf@latest
```

## One-shot setup

```bash
git clone https://github.com/AS203038/looking-glass.git
cd looking-glass
make install     # Go modules + pnpm workspace + protobuf npm deps
make build       # webui + server + CLI
cp example.config.yaml config.yaml
$EDITOR config.yaml
./looking-glass
```

`make help` always lists every target with one-line descriptions.

## Local development loop

The two surfaces — Go backend and SvelteKit frontend — develop
independently. There is no need to rebuild the entire embed
pipeline on every edit.

### Backend only (most common)

```bash
make dev-server       # runs: go run ./cmd/server
```

The first run also creates an empty `cmd/server/dist/` (so the
`//go:embed all:dist` directive doesn't fail). The WebUI won't be
served via the embed in this mode — you either don't need it or
you're running the Vite dev server in parallel.

Edit Go code, hit Ctrl-C, rerun. There is no built-in hot-reload;
use [`air`](https://github.com/cosmtrek/air) or
[`reflex`](https://github.com/cespare/reflex) if you want one:

```bash
go install github.com/air-verse/air@latest
air -c .air.toml     # if you write one; the project doesn't ship a default
```

### Frontend only

```bash
make dev-webui       # cd webui && pnpm run dev
```

Vite serves the UI on <http://localhost:5173/> with HMR. To make it
talk to your local backend, add to `config.yaml`:

```yaml
web:
  grpc_url: "http://localhost:8080"
```

…or override at the env level (the WebUI reads
`$env/dynamic/public`):

```bash
PUBLIC_GRPC_URL=http://localhost:8080 pnpm --dir webui dev
```

CORS is permissive for development (the server's CORS middleware
allows `*` by default), so cross-origin gRPC-Web requests from
`localhost:5173` to `localhost:8080` work out of the box.

### Full-stack dev

Run both servers side by side, in different terminals:

```bash
# Terminal 1
make dev-server

# Terminal 2
make dev-webui
```

Open <http://localhost:5173/> for the dev UI. The embedded UI at
<http://localhost:8080/> will be empty/stale until you `make
build-webui` — that's fine, the dev server gives you the latest.

### Producing a final artefact

```bash
make build           # webui → cmd/server/dist → go build → ./looking-glass + ./lg-cli
./looking-glass
```

A clean source tree should always produce a working binary with
`make build`. CI runs the same command path.

## Protobuf workflow

The single source of truth is
[`protobuf/lookingglass/v0/lookingglass.proto`](../protobuf/lookingglass/v0/lookingglass.proto).

```bash
make proto           # regenerate Go + TS clients
make proto-lint      # buf lint
make proto-format    # buf format -w (in-place)
```

`make proto` runs `cd protobuf && buf generate` which produces:

* `protobuf/lookingglass/v0/lookingglass.pb.go`
* `protobuf/lookingglass/v0/lookingglassconnect/lookingglass.connect.go`
* `protobuf/lookingglass/v0/lookingglass_pb.ts` (messages **and**
  the service descriptor `LookingGlassService` — protobuf-es v2
  collapsed the separate `_connect.ts` from v1 into the same
  module)

The TS output is consumed by the WebUI workspace via the
`@as203038/lg-protobuf` workspace link (`workspace:*` in
`webui/package.json`).

**Comments propagate**: any prose you write on a `service`, `rpc`,
`message`, or field in the `.proto` file becomes godoc on the
generated Go and TSDoc on the generated TS. Treat the `.proto` as
the API documentation.

When you change the proto:

1. Edit the `.proto`.
2. `make proto`.
3. Update server (`pkg/http/grpc/service.go`) and consumers
   (`webui/src/lib/stores/query.ts`, `cmd/cli/cmd_*.go`) as needed.
4. Run `make check` to catch type / lint errors across the whole
   tree.

The service stays at `v0` — we treat it as field-additive
indefinitely. Removing or renaming fields is a breaking change
that needs a `v1` package; introduce it explicitly when the time
comes.

## Testing

```bash
make test            # go test ./...
make test-cover      # go test -cover ./...
```

There aren't many tests today; this is a known shortcoming and
contributions are extremely welcome. When adding tests:

* Keep them next to the file they cover (`foo.go` → `foo_test.go`).
* Don't network. Use `iotest` / fakes / interfaces.
* For SSH-dependent paths, factor the SSH-execution into an
  interface and stub it (the `utils.Router` interface is already
  a natural seam).
* Use the standard `testing` package; no third-party assertion
  library is required.

### Frontend checks

```bash
make check-webui     # svelte-check (TypeScript + Svelte)
make lint-webui      # prettier + eslint
make format-webui    # prettier in-place
```

`pnpm check` returns `0 errors, 0 warnings` on a clean tree. The
build itself (`make build-webui`) is the final source of truth: if
adapter-static can't render the routes, the tree is broken.

### CI parity

The full pre-push sequence is `make check`, which runs
`make lint test`. Match what CI runs locally by sticking with
`make` targets.

```bash
make check           # lint + test
```

## Project layout

```
cmd/
├── server/          # `looking-glass` daemon — main.go is ~60 LoC
└── cli/             # `lg-cli` — one file per cobra subcommand

pkg/
├── errs/            # Sentinel errors (one file per concern)
├── http/            # HTTP/2 listener + middleware chain
│   ├── grpc/        # ConnectRPC service + health checker
│   └── webui/       # /_app/env.js runtime-config injector
├── routers/         # Template registry + bundled vendor *.yml
└── utils/           # Config, IP/CIDR, SSH, sanitize, TLS, version

protobuf/            # buf-managed proto + generated Go/TS clients

webui/
├── src/
│   ├── lib/         # Reusable building blocks
│   │   ├── stores/  # Svelte stores (routers, query, sheet)
│   │   ├── components/  # Visual components (CommandDock, ResultsSheet, …)
│   │   ├── env.ts   # Wraps $env/dynamic/public with defaults
│   │   └── grpc.ts  # Singleton ConnectRPC client
│   ├── routes/      # SvelteKit pages and layouts
│   ├── hooks.client.ts  # Sentry lazy-init
│   └── app.html     # HTML shell (pre-paint theme script lives here)
├── static/          # Favicons, manifest, robots.txt
├── svelte.config.js # adapter-static → ../cmd/server/dist
└── package.json     # workspace:* to ../protobuf

docs/                # ← this documentation
example.config.yaml  # Annotated reference config
public_index.yaml    # Registry of public LG instances
Makefile             # Local tasks
Dockerfile           # Multi-stage build
go.mod               # Single Go module: github.com/AS203038/looking-glass
```

## Coding conventions

### Go

* **godoc on every exported identifier.** Start the comment with
  the identifier name. Package-level comments live on the first
  Go file in the package (or a `doc.go` if you prefer).
* **`go fmt` strictly.** `make fmt-go` runs `gofmt -s -w cmd pkg`.
* **No third-party logging.** The standard `log` package is the
  only logger; no `zap`, no `logrus`, no `zerolog`. Operator log
  lines should be greppable (one line per event, fixed-shape tags
  like `op=… router=…`).
* **Sentinel errors over typed errors.** Add new entries to
  `pkg/errs/` rather than introducing struct-typed errors that
  carry sensitive data.
* **No panics outside startup.** `init()` and `register()` may
  panic; nothing reachable from a request handler may.
* **Stateless handlers.** No package-level mutable globals in
  request paths. The only legitimate globals are: the router
  registry (`_routers`), the Redis client (initialised once at
  startup), and the Sentry hub (initialised once).
* **Concurrency:** explicit goroutines are fine; channels are
  preferred over mutexes; sync.Mutex is a last resort.

### TypeScript / Svelte

* **Svelte 5 runes.** Use `$state`, `$derived`, `$effect` in new
  components; the legacy reactive-statement style is deprecated.
* **Stores own all cross-component state.** Components subscribe;
  components do not write to other components' stores.
* **Singleton ConnectRPC client.** `webui/src/lib/grpc.ts` exports
  the only client. Don't construct your own.
* **Tailwind utility classes**, not raw CSS. Theme tokens are
  CSS custom properties; refer to them via `bg-(--color-bg)`
  rather than mode-specific colour names. Tailwind v4 `@theme`
  blocks in `app.css` define both Latte (light) and Mocha (dark)
  palettes.
* **Icons:** per-icon tree-shaken imports from `@lucide/svelte`
  (`import Foo from "@lucide/svelte/icons/foo"`). Lucide doesn't
  ship every brand mark; use `Code` as a fallback.
* **Prettier + ESLint.** `make lint-webui` is the gate.

### YAML (router templates)

See [router-templates.md § Style guide](./router-templates.md#style-guide).

### Commits & PRs

* One concern per commit. Keep changes focused.
* Commit messages: imperative mood, ≤72 chars subject, blank line,
  body wrapped at 72 chars. Reference issues in the body.
* PRs that touch the WebUI must rebuild and commit
  `cmd/server/dist/`? **No.** `cmd/server/dist/` is `.gitignore`d
  and built by CI. The only thing the embed needs at PR time is a
  placeholder; the Makefile creates `cmd/server/dist/.gitkeep`
  automatically.
* **AI-generated contributions** are accepted on a case-by-case
  basis but are *not* the primary workflow. If a change was
  produced or substantially shaped by an AI assistant, say so
  in the PR description — the maintainers will review with
  appropriate scrutiny. Human review and the usual
  quality bar (`make check && make build`, sensible commit
  history, no hallucinated APIs) still apply.

## Versioning

`utils.Version()` returns `<release>+<unix-nanos-hex>`. The
`<release>` part is set at build time via:

```bash
go build -ldflags "-X github.com/AS203038/looking-glass/pkg/utils.release=v1.2.3" \
  -o looking-glass ./cmd/server
```

The Makefile does this automatically using `git describe` output;
override with `make build VERSION=v1.2.3`. The Dockerfile takes
the version as a build arg.

The CLI's `lg-cli version` subcommand reads `main.Version`, set
via `-X main.Version=v1.2.3`. The Makefile handles both.

## Releasing

This project uses GitHub Actions for releases. The shape (subject
to change — read `.github/workflows/release.yaml` for the truth):

1. Tag a release: `git tag -a v1.2.3 -m "1.2.3"` and push.
2. The `release.yaml` workflow:
   * Builds the protobuf with `buf generate`.
   * Builds the WebUI with `pnpm install --frozen-lockfile && pnpm
     run build`.
   * Builds Go binaries for several `GOOS/GOARCH` combinations
     with the appropriate `-ldflags`.
   * Uploads them to a GitHub Release alongside
     `example.config.yaml`.
3. The Docker workflow (or `docker-release` job) builds and pushes
   `ghcr.io/as203038/looking-glass:v1.2.3` and `:latest`.

There is no separate npm publish for the protobuf package — it's
workspace-linked from the WebUI and bundled into the static
build.

## Common workflows

### Adding a new gRPC method

1. Edit `protobuf/lookingglass/v0/lookingglass.proto`: add `rpc
   Foo(...) returns (...)` and any new messages.
2. `make proto`.
3. Implement the method on `pkg/http/grpc/service.go`. Follow the
   existing pattern: router lookup → input sanitisation →
   template render → SSH exec → response assembly → coarse
   sentinel errors.
4. If the operation needs a new template section, extend
   `pkg/routers/yaml.go` (`Yaml.Template`, `_tpl_data`, `_tpl`
   switch case, new method) and update every bundled `*.yml`.
5. Add a CLI subcommand in `cmd/cli/cmd_*.go`. Use the existing
   `client.Bar` calls as a template.
6. Add a WebUI button + store dispatch in `webui/src/lib/stores/query.ts`
   and `CommandDock.svelte`.
7. Run `make check`.

### Adding a new bundled router template

See [router-templates.md § Authoring a new template](./router-templates.md#authoring-a-new-template).
Drop the file in `pkg/routers/<vendor>_<model>.yml` instead of
`$ROUTER_DIR` so it ends up in the binary.

### Changing the WebUI runtime env

1. Add the field to `pkg/http/webui/webui.go`'s `EnvJS` struct
   (with the `PUBLIC_*` JSON tag).
2. Wire it from `utils.WebConfig` (you'll likely need to add a new
   YAML key too).
3. Add the matching field to `webui/src/lib/env.ts` (`PublicEnv`
   interface + `defaults` object + any coercion).
4. Use it in the WebUI.
5. Document the new YAML key in
   [configuration.md](./configuration.md).

### Tweaking middleware behaviour

Everything lives in `pkg/http/http.go`. Middleware is composed
bottom-up in `ListenAndServe`; insertion order matters. The
existing structure (cache → cors → log → sentry) reflects a
deliberate choice — see [architecture.md](./architecture.md).

When adding new middleware:

* Keep it idempotent (anything reasonable for HTTP).
* Don't read/write shared state without explanation.
* Log at the same prefix style as the rest of the codebase.

## Reviewing your changes locally

Before opening a PR, the closest-to-CI sequence is:

```bash
make generate        # if you touched the proto
make check           # lint + test
make build           # full build
```

A green `make check && make build` is the bar.

## Where to ask questions

* GitHub Issues / Discussions: <https://github.com/AS203038/looking-glass>
* The demo instance at <https://lg.as203038.net/> is run by the
  primary maintainer; production-shaped questions can include
  references to its behaviour.

If you're stuck on a router template, the maintainer can often be
persuaded to provide read-only access to the AS203038 demo
routers — file an issue.

## License

GPL-3.0-or-later. By contributing, you agree to license your work
under the same terms.
