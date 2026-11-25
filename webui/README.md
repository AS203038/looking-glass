# Looking Glass WebUI

SvelteKit 5 static web application for [AS203038/looking-glass](https://github.com/AS203038/looking-glass). It builds into `../cmd/server/dist` and is embedded directly into the Go server binary.

## Architecture

- **Framework**: SvelteKit 3 + Svelte 5 runes (`runes: true`), Tailwind CSS v4 (Catppuccin Latte & Mocha).
- **Transport**: ConnectRPC gRPC-Web (`@connectrpc/connect-web`) using binary protobuf payloads from `@as203038/lg-protobuf`.
- **Runtime Config**: Read from `/_app/env.js` (`$app/env/public`) served by `pkg/http/webui` so a single static build works across environments.
- **State & Views**:
  - `src/lib/stores/routers.ts` — paginated router catalogue, periodic health refresh, and selection state.
  - `src/lib/stores/query.ts` — command execution, bounded retries, abort support, peer-route pagination, and interactive pivoting.
  - `src/lib/stores/history.ts` — localStorage-backed execution history (last 15 runs with quota recovery).
  - `src/lib/stores/sheet.ts` — resizable bottom results sheet state.
  - `src/lib/components/views/` — structured and diff views for Ping, Traceroute, BGP Summary, and BGP Paths.

## Runtime Environment Variables

| Variable                    | Default                  | Description                                                                             |
| :-------------------------- | :----------------------- | :-------------------------------------------------------------------------------------- |
| `PUBLIC_PAGE_TITLE`         | `"Looking Glass"`        | Browser tab `<title>`.                                                                  |
| `PUBLIC_HEADER_TEXT`        | `"Looking Glass"`        | Header brand label.                                                                     |
| `PUBLIC_HEADER_LOGO`        | `""`                     | Header logo URL.                                                                        |
| `PUBLIC_HEADER_LINKS`       | `""`                     | Comma-separated `name\|href` pairs (safe schemes only: `http`, `https`, `mailto`, `/`). |
| `PUBLIC_FOOTER_TEXT`        | `""`                     | Footer brand label.                                                                     |
| `PUBLIC_FOOTER_LOGO`        | `""`                     | Footer logo URL.                                                                        |
| `PUBLIC_FOOTER_LINKS`       | `""`                     | Comma-separated `name\|href` pairs.                                                     |
| `PUBLIC_GRPC_URL`           | `window.location.origin` | Base URL for gRPC-Web requests.                                                         |
| `PUBLIC_LG_VERSION`         | `"dev"`                  | Server release version displayed in the footer.                                         |
| `PUBLIC_SENTRY_DSN`         | `""`                     | Enables `@sentry/svelte` when non-empty.                                                |
| `PUBLIC_SENTRY_ENV`         | `"dev"`                  | Sentry environment tag.                                                                 |
| `PUBLIC_SENTRY_SAMPLE_RATE` | `0`                      | Sentry tracing sample rate (`0`–`1`).                                                   |
| `PUBLIC_SENTRY_REPLAY`      | `false`                  | Enables Sentry Session Replay when `true`.                                              |

## Development & Verification

```sh
pnpm install --frozen-lockfile
pnpm dev          # Start Vite dev server
pnpm check        # Run svelte-kit sync + svelte-check
pnpm lint         # Run Prettier check + ESLint
pnpm test         # Run Vitest suite
pnpm build        # Emit static bundle to ../cmd/server/dist
```
