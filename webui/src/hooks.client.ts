import { getEnv } from '$lib/env';

const env = getEnv();

if (env.PUBLIC_SENTRY_DSN) {
	import('@sentry/svelte').then(({ init, browserTracingIntegration, replayIntegration }) => {
		init({
			dsn: env.PUBLIC_SENTRY_DSN,
			environment: env.PUBLIC_SENTRY_ENV,
			release: env.PUBLIC_LG_VERSION?.split('+')[0],
			integrations: [browserTracingIntegration(), replayIntegration()],
			tracesSampleRate: env.PUBLIC_SENTRY_SAMPLE_RATE,
			replaysSessionSampleRate: 0.1,
			replaysOnErrorSampleRate: 1.0
		});
	});
}
