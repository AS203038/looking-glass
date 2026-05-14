/**
 * Client-side Sentry bootstrap.
 *
 * Runs once during initial hydration. We feature-detect: if no DSN is
 * configured (most installs), this is a no-op and Sentry never loads.
 */

import { getEnv } from '$lib/env';

const env = getEnv();

if (env.PUBLIC_SENTRY_DSN) {
	// Dynamic import keeps Sentry out of the initial bundle for the
	// (common) case where it isn't configured.
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
