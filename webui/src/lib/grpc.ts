import { LookingGlassService } from "@as203038/lg-protobuf/lookingglass/v0/lookingglass_connect";
export type * as Pb from "@as203038/lg-protobuf/lookingglass/v0/lookingglass_pb";
import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { createPromiseClient, type PromiseClient } from "@connectrpc/connect";
import { env } from "$env/dynamic/public";

let client: PromiseClient<typeof LookingGlassService> | null = null;

export const LookingGlassClient = () => {
  if (!client) {
    client = createPromiseClient<typeof LookingGlassService>(
      LookingGlassService,
      createGrpcWebTransport({
        baseUrl: env.PUBLIC_GRPC_URL,
        useBinaryFormat: true,
      }) as any,
    ); // Add 'as any' to bypass type checking
  }
  return client;
};
