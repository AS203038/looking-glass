import { LookingGlassService } from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';
import { createClient, type Client } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { getEnv } from './env';

export type * as Pb from '@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb';

let client: Client<typeof LookingGlassService> | null = null;

/** Returns the lazily-constructed singleton ConnectRPC client. */
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
