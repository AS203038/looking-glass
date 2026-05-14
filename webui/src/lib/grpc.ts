/**
 * Singleton ConnectRPC client for the LookingGlassService.
 *
 * Uses gRPC-Web binary transport so it works over plain HTTP/1.1 in front
 * of a Connect/h2c backend (the deployed default) without requiring HTTP/2
 * end-to-end. Created lazily so SSR/prerender passes that touch this file
 * don't try to construct a transport at module-load time.
 *
 * Wired against Protobuf-ES v2 / Connect-ES v2: the service descriptor
 * (`LookingGlassService`) is now exported from the generated `_pb` module
 * alongside the messages — the separate `_connect` codegen plugin has been
 * retired by upstream. `createClient` replaces v1's `createPromiseClient`,
 * and `Client<T>` replaces `PromiseClient<T>`; both still live in
 * `@connectrpc/connect`.
 */

import { LookingGlassService } from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import { createClient, type Client } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { getEnv } from './env';

export type * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';

let client: Client<typeof LookingGlassService> | null = null;

export function LookingGlassClient(): Client<typeof LookingGlassService> {
	if (!client) {
		const { PUBLIC_GRPC_URL } = getEnv();
		client = createClient(
			LookingGlassService,
			createGrpcWebTransport({ baseUrl: PUBLIC_GRPC_URL, useBinaryFormat: true })
		);
	}
	return client;
}
