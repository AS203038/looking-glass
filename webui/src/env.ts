import { defineEnvVars } from '@sveltejs/kit/env';

const optional = (value: string | undefined) => value ?? '';

export const variables = defineEnvVars({
	PUBLIC_PAGE_TITLE: { public: true, schema: optional },
	PUBLIC_HEADER_TEXT: { public: true, schema: optional },
	PUBLIC_HEADER_LINKS: { public: true, schema: optional },
	PUBLIC_HEADER_LOGO: { public: true, schema: optional },
	PUBLIC_FOOTER_TEXT: { public: true, schema: optional },
	PUBLIC_FOOTER_LINKS: { public: true, schema: optional },
	PUBLIC_FOOTER_LOGO: { public: true, schema: optional },
	PUBLIC_GRPC_URL: { public: true, schema: optional },
	PUBLIC_LG_VERSION: { public: true, schema: optional },
	PUBLIC_SENTRY_DSN: { public: true, schema: optional },
	PUBLIC_SENTRY_ENV: { public: true, schema: optional },
	PUBLIC_SENTRY_SAMPLE_RATE: { public: true, schema: optional },
	PUBLIC_SENTRY_REPLAY: { public: true, schema: optional }
});
