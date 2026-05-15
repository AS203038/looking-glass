import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	compilerOptions: {
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
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
