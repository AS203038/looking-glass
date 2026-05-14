/**
 * Query store: which command + parameter the user has chosen, plus the
 * per-router result lifecycle for the most recent submission.
 *
 * One submission produces N parallel router executions; their state is
 * keyed by router-id stringified (bigint → string) because JS Map keys
 * compare by identity for objects/bigints which makes derived lookups
 * brittle.
 */

import { writable, get } from 'svelte/store';
import { LookingGlassClient, type Pb } from '$lib/grpc';
import { selectedRouters } from './routers';
import { onRunStarted } from './sheet';

export const COMMANDS = [
	{ value: 'ping', label: 'Ping', placeholder: 'IPv4 or IPv6 address' },
	{ value: 'traceroute', label: 'Traceroute', placeholder: 'IPv4 or IPv6 address' },
	{ value: 'bgp_route', label: 'BGP Route', placeholder: 'IPv4/IPv6 prefix or address' },
	{
		value: 'bgp_community',
		label: 'BGP Community',
		placeholder: 'ASN:VALUE or GLOBAL:LOCAL1:LOCAL2'
	},
	{ value: 'bgp_aspath_regex', label: 'BGP AS-Path Regex', placeholder: '^65000_ .* _65001$' }
] as const;

export type CommandValue = (typeof COMMANDS)[number]['value'];

export type ExecStatus = 'pending' | 'running' | 'done' | 'error';

export interface ExecResult {
	routerId: string;
	routerName: string;
	routerLocation: string;
	status: ExecStatus;
	timestamp: Date | null;
	/** Raw response bytes, decoded lazily for display. Empty for 'error'. */
	bytes: Uint8Array | null;
	/** Populated only for 'error'. */
	error: string | null;
	/** Sub-command resolved for this submission (e.g. 'bgp_large_community' under bgp_community). */
	resolvedCommand: string;
}

export const command = writable<CommandValue | ''>('');
export const parameter = writable<string>('');
/** Last submission's results, keyed by routerId.toString(). */
export const results = writable<Record<string, ExecResult>>({});
/** Whether at least one submission has happened in this session. */
export const hasRun = writable<boolean>(false);

/** Detect the auto-resolved subcommand for the special bgp_community case. */
export function resolveCommand(cmd: CommandValue, param: string): string {
	if (cmd === 'bgp_community') {
		const parts = param.split(':');
		if (parts.length === 2) return 'bgp_community';
		if (parts.length === 3) return 'bgp_large_community';
	}
	return cmd;
}

async function runOne(router: Pb.Router, cmd: CommandValue, param: string): Promise<ExecResult> {
	const key = router.id.toString();
	const base: ExecResult = {
		routerId: key,
		routerName: router.name,
		routerLocation: router.location ?? '',
		status: 'running',
		timestamp: null,
		bytes: null,
		error: null,
		resolvedCommand: resolveCommand(cmd, param)
	};
	results.update((r) => ({ ...r, [key]: base }));

	const client = LookingGlassClient();
	try {
		let res:
			| Pb.PingResponse
			| Pb.TracerouteResponse
			| Pb.BGPRouteResponse
			| Pb.BGPCommunityResponse
			| Pb.BGPLargeCommunityResponse
			| Pb.BGPASPathResponse;

		switch (cmd) {
			case 'ping':
				res = await client.ping({ routerId: router.id, target: param });
				break;
			case 'traceroute':
				res = await client.traceroute({ routerId: router.id, target: param });
				break;
			case 'bgp_route':
				res = await client.bGPRoute({ routerId: router.id, target: param });
				break;
			case 'bgp_community': {
				const parts = param.split(':');
				if (parts.length === 2) {
					res = await client.bGPCommunity({
						routerId: router.id,
						community: { asn: parseInt(parts[0], 10), value: parseInt(parts[1], 10) }
					});
				} else if (parts.length === 3) {
					res = await client.bGPLargeCommunity({
						routerId: router.id,
						community: {
							globalAdmin: parseInt(parts[0], 10),
							localData1: parseInt(parts[1], 10),
							localData2: parseInt(parts[2], 10)
						}
					});
				} else {
					throw new Error(
						`Invalid community "${param}": expected ASN:VALUE or GLOBAL:LOCAL1:LOCAL2`
					);
				}
				break;
			}
			case 'bgp_aspath_regex':
				res = await client.bGPASPath({ routerId: router.id, pattern: param });
				break;
			default:
				throw new Error(`Unknown command: ${cmd}`);
		}

		const tsSeconds = res.timestamp?.seconds ? Number(res.timestamp.seconds) : Date.now() / 1000;
		const done: ExecResult = {
			...base,
			status: 'done',
			bytes: res.result,
			timestamp: new Date(tsSeconds * 1000)
		};
		results.update((r) => ({ ...r, [key]: done }));
		return done;
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		const failed: ExecResult = {
			...base,
			status: 'error',
			error: message,
			timestamp: new Date()
		};
		results.update((r) => ({ ...r, [key]: failed }));
		console.error(`[${router.name}] ${cmd} failed:`, err);
		return failed;
	}
}

/** Submit the current command + parameter against the currently-selected routers. */
export async function run(cmd: CommandValue, param: string) {
	const routers = get(selectedRouters);
	if (routers.length === 0 || !cmd || !param) return;

	hasRun.set(true);
	// Pressing Execute is an explicit request to see results: open the
	// sheet immediately (every time), so the user sees pending → running
	// → done lifecycle as it happens. Decoupled from `hasRun` so this
	// fires on every submission, not just the first one.
	onRunStarted();
	// Seed all rows as pending so the UI snaps into the result layout
	// immediately, before any network round-trip completes.
	const seed: Record<string, ExecResult> = {};
	for (const r of routers) {
		seed[r.id.toString()] = {
			routerId: r.id.toString(),
			routerName: r.name,
			routerLocation: r.location ?? '',
			status: 'pending',
			timestamp: null,
			bytes: null,
			error: null,
			resolvedCommand: resolveCommand(cmd, param)
		};
	}
	results.set(seed);

	await Promise.all(routers.map((r) => runOne(r, cmd, param)));
}
