// Static-adapter SPA: we have a runtime env served by the Go backend,
// so prerendering would produce a stale env snapshot. Render fully client-side.
export const prerender = false;
export const ssr = false;
