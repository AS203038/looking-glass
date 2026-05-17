import { writable, get } from 'svelte/store';
import { LookingGlassClient, type Pb } from '$lib/grpc';
import { selectedRouters } from './routers';
import { onRunStarted } from './sheet';
import { pushHistory } from './history';

export const COMMANDS = [
	{ value: 'ping', label: 'Ping', placeholder: 'IPv4 or IPv6 address' },
	{ value: 'traceroute', label: 'Traceroute', placeholder: 'IPv4 or IPv6 address' },
	{ value: 'bgp_summary', label: 'BGP Summary', placeholder: '(no parameter required)' },
	{ value: 'bgp_route', label: 'BGP Route', placeholder: 'IPv4/IPv6 prefix or address' },
	{
		value: 'bgp_community',
		label: 'BGP Community',
		placeholder: 'ASN:VALUE or GLOBAL:LOCAL1:LOCAL2'
	},
	{ value: 'bgp_aspath_regex', label: 'BGP AS-Path Regex', placeholder: '^65000_ .* _65001$' }
] as const;

export type CommandValue = (typeof COMMANDS)[number]['value'];

/** Commands that accept no parameter. */
export const COMMANDS_NO_PARAM: ReadonlyArray<CommandValue> = ['bgp_summary'];

export type ExecStatus = 'pending' | 'running' | 'done' | 'error';

/** Parsed payload variants carried alongside each result. */
export type ParsedPayload =
	| { kind: 'ping'; data: Pb.PingStats }
	| { kind: 'traceroute'; data: Pb.TracerouteParsed }
	| { kind: 'bgp_summary'; data: Pb.BGPSummaryParsed }
	| { kind: 'bgp_paths'; data: Pb.BGPPaths };

export interface ExecResult {
	routerId: string;
	routerName: string;
	routerLocation: string;
	status: ExecStatus;
	timestamp: Date | null;
	/** Raw response bytes, decoded lazily for display. */
	bytes: Uint8Array | null;
	/** Populated only when status is 'error'. */
	error: string | null;
	/** Original user command. */
	queryCommand: CommandValue;
	/** Original user parameter. */
	queryParameter: string;
	/** Sub-command resolved for this submission. */
	resolvedCommand: string;
	/** Structured parser payload, when the server populated one. */
	parsed: ParsedPayload | null;
	/** Server-reported parser provenance. */
	parserKind: Pb.ParserKind;
	/** Server-reported parse outcome. */
	parseStatus: Pb.ParseStatus;
	/** True if the result was served from the backend cache. */
	cached: boolean;
}

export const command = writable<CommandValue | ''>('');
export const parameter = writable<string>('');
/** Last submission's results, keyed by routerId.toString(). */
export const results = writable<Record<string, ExecResult>>({});
/** Whether at least one submission has happened in this session. */
export const hasRun = writable<boolean>(false);

/** Returns the resolved subcommand, auto-detecting standard vs large community. */
export function resolveCommand(cmd: CommandValue, param: string): string {
	if (cmd === 'bgp_community') {
		const parts = param.split(':');
		if (parts.length === 2) return 'bgp_community';
		if (parts.length === 3) return 'bgp_large_community';
	}
	return cmd;
}

/** Narrows an operation response into a [ParsedPayload], or null when unavailable. */
function pickParsed(
	cmd: CommandValue,
	res:
		| Pb.PingResponse
		| Pb.TracerouteResponse
		| Pb.BGPSummaryResponse
		| Pb.BGPRouteResponse
		| Pb.BGPCommunityResponse
		| Pb.BGPLargeCommunityResponse
		| Pb.BGPASPathResponse
): ParsedPayload | null {
	const ok = (res as { parseStatus?: number }).parseStatus === 0;
	if (!ok) return null;
	switch (cmd) {
		case 'ping': {
			const d = (res as Pb.PingResponse).parsed;
			return d ? { kind: 'ping', data: d } : null;
		}
		case 'traceroute': {
			const d = (res as Pb.TracerouteResponse).parsed;
			return d ? { kind: 'traceroute', data: d } : null;
		}
		case 'bgp_summary': {
			const d = (res as Pb.BGPSummaryResponse).parsed;
			return d ? { kind: 'bgp_summary', data: d } : null;
		}
		case 'bgp_route':
		case 'bgp_community':
		case 'bgp_aspath_regex': {
			const d = (
				res as
					| Pb.BGPRouteResponse
					| Pb.BGPCommunityResponse
					| Pb.BGPLargeCommunityResponse
					| Pb.BGPASPathResponse
			).parsed;
			return d ? { kind: 'bgp_paths', data: d } : null;
		}
	}
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
		queryCommand: cmd,
		queryParameter: param,
		resolvedCommand: resolveCommand(cmd, param),
		parsed: null,
		parserKind: 0,
		parseStatus: 1,
		cached: false
	};
	results.update((r) => ({ ...r, [key]: base }));

	const client = LookingGlassClient();
	let isCached = false;
	const callOptions = {
		onHeader: (headers: Headers) => {
			if (headers.get('x-cache') === 'HIT') {
				isCached = true;
			}
		}
	};

	try {
		let res:
			| Pb.PingResponse
			| Pb.TracerouteResponse
			| Pb.BGPSummaryResponse
			| Pb.BGPRouteResponse
			| Pb.BGPCommunityResponse
			| Pb.BGPLargeCommunityResponse
			| Pb.BGPASPathResponse;

		switch (cmd) {
			case 'ping':
				res = await client.ping({ routerId: router.id, target: param }, callOptions);
				break;
			case 'traceroute':
				res = await client.traceroute({ routerId: router.id, target: param }, callOptions);
				break;
			case 'bgp_summary':
				res = await client.bGPSummary({ routerId: router.id }, callOptions);
				break;
			case 'bgp_route':
				res = await client.bGPRoute({ routerId: router.id, target: param }, callOptions);
				break;
			case 'bgp_community': {
				const parts = param.split(':');
				if (parts.length === 2) {
					res = await client.bGPCommunity(
						{
							routerId: router.id,
							community: { asn: parseInt(parts[0], 10), value: parseInt(parts[1], 10) }
						},
						callOptions
					);
				} else if (parts.length === 3) {
					res = await client.bGPLargeCommunity(
						{
							routerId: router.id,
							community: {
								globalAdmin: parseInt(parts[0], 10),
								localData1: parseInt(parts[1], 10),
								localData2: parseInt(parts[2], 10)
							}
						},
						callOptions
					);
				} else {
					throw new Error(
						`Invalid community "${param}": expected ASN:VALUE or GLOBAL:LOCAL1:LOCAL2`
					);
				}
				break;
			}
			case 'bgp_aspath_regex':
				res = await client.bGPASPath({ routerId: router.id, pattern: param }, callOptions);
				break;
			default:
				throw new Error(`Unknown command: ${cmd}`);
		}

		const tsSeconds = res.timestamp?.seconds ? Number(res.timestamp.seconds) : Date.now() / 1000;
		const done: ExecResult = {
			...base,
			status: 'done',
			bytes: res.result,
			timestamp: new Date(tsSeconds * 1000),
			parsed: pickParsed(cmd, res),
			parserKind: (res as { parserKind?: Pb.ParserKind }).parserKind ?? 0,
			parseStatus: (res as { parseStatus?: Pb.ParseStatus }).parseStatus ?? 1,
			cached: isCached
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

/** Submits the current command and parameter against the selected routers. */
export async function run(cmd: CommandValue, param: string) {
	const routers = get(selectedRouters);
	if (routers.length === 0 || !cmd) return;
	if (!COMMANDS_NO_PARAM.includes(cmd) && !param) return;

	hasRun.set(true);
	onRunStarted();
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
			queryCommand: cmd,
			queryParameter: param,
			resolvedCommand: resolveCommand(cmd, param),
			parsed: null,
			parserKind: 0,
			parseStatus: 1,
			cached: false
		};
	}
	results.set(seed);

	await Promise.all(routers.map((r) => runOne(r, cmd, param)));

	pushHistory(cmd, param, routers, get(results));
}
