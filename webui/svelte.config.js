import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	compilerOptions: {
		// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		// SPA mode embedded into the Go binary. The backend serves the SPA
		// shell + a runtime `/_app/env.js` injected from config (see
		// `pkg/http/webui/webui.go`), so:
		//   - `pages` / `assets` write directly into `cmd/server/dist`,
		//     where Go's `//go:embed all:dist` picks them up at compile time.
		//   - `fallback: index.html` makes any unknown route resolve to the
		//     SPA shell (we only have `/` today, but this keeps deep-links
		//     working when we add routes later).
		//   - `strict: false` lets us coexist with the runtime env.js stub
		//     served at `/_app/env.js` (which doesn't exist at build time).
		adapter: adapter({
			pages: '../cmd/server/dist',
			assets: '../cmd/server/dist',
			fallback: 'index.html',
			precompress: false,
			strict: false
		})
	}
};

export default config;
