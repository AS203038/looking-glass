import adapter from '@sveltejs/adapter-static';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			alias: {
				$lib: 'src/lib'
			},
			adapter: adapter({
				pages: '../cmd/server/dist',
				assets: '../cmd/server/dist',
				fallback: 'index.html',
				precompress: false,
				strict: false
			})
		}),
		svelteTesting()
	],
	test: {
		environment: 'jsdom',
		setupFiles: ['./src/test/setup.ts']
	}
});
