/**
 * Barrel module for the `$lib` alias.
 *
 * SvelteKit resolves the `$lib` import alias to this directory.
 * Files placed under `src/lib/` can be imported from anywhere in
 * the app via `$lib/<file>`; that mechanism does not require this
 * index to re-export them, so the file is intentionally empty.
 *
 * Add explicit re-exports here only when curating a small public
 * surface (e.g. for documentation or to break a circular import);
 * for everyday use prefer direct `$lib/path` imports.
 */

export {};
