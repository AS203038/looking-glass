/**
 * Singleton ConnectRPC client for the LookingGlassService.
 *
 * Uses gRPC-Web binary transport so it works over plain HTTP/1.1 in front
 * of a Connect/h2c backend (the deployed default) without requiring HTTP/2
 * end-to-end. Created lazily so SSR/prerender passes that touch this file
 * don't try to construct a transport at module-load time.
 */

import { LookingGlassService } from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_connect';
import { createPromiseClient, type PromiseClient } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { getEnv } from './env';

export type * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';

let client: PromiseClient<typeof LookingGlassService> | null = null;

export function LookingGlassClient(): PromiseClient<typeof LookingGlassService> {
	if (!client) {
		const { PUBLIC_GRPC_URL } = getEnv();
		client = createPromiseClient(
			LookingGlassService,
			createGrpcWebTransport({ baseUrl: PUBLIC_GRPC_URL, useBinaryFormat: true })
		);
	}
	return client;
}
