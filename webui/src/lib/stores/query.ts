import { writable, get } from 'svelte/store';
import { LookingGlassClient, Pb } from '$lib/grpc';
import { ConnectError, Code } from '@connectrpc/connect';
import {
	clearSelection,
	routers as catalogueRouters,
	selectedRouters,
	selectMany
} from './routers';
import { onRunStarted } from './sheet';
import { pushHistory, type HistoryEntry } from './history';
import { pushToast } from '$lib/toast';

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
	{ value: 'bgp_aspath_regex', label: 'BGP AS-Path Regex', placeholder: '^65000_ .* _65001$' },
	{
		value: 'bgp_peer_routes',
		label: 'BGP Peer Routes',
		placeholder: 'peer_ip [received|accepted|rejected|advertised] [peer_name]'
	}
] as const;

export type CommandValue = (typeof COMMANDS)[number]['value'];

export type PeerRouteQueryTypeName = 'received' | 'accepted' | 'rejected' | 'advertised';

const PEER_ROUTE_TYPES: ReadonlySet<string> = new Set<PeerRouteQueryTypeName>([
	'received',
	'accepted',
	'rejected',
	'advertised'
]);

/** Commands that accept no parameter. */
export const COMMANDS_NO_PARAM: ReadonlyArray<CommandValue> = ['bgp_summary'];

export type ExecStatus = 'pending' | 'running' | 'done' | 'error';

export type ErrorKind = 'error' | 'timeout' | 'canceled';

/** Formats an RPC or client error into a user-facing message and category. */
export function formatRpcError(
	err: unknown,
	aborted = false
): { message: string; kind: ErrorKind } {
	if (
		aborted ||
		(err instanceof DOMException && err.name === 'AbortError') ||
		(err instanceof ConnectError && err.code === Code.Canceled)
	) {
		return { message: 'Canceled', kind: 'canceled' };
	}
	if (err instanceof ConnectError && err.code === Code.DeadlineExceeded) {
		return {
			message: err.rawMessage ? `Command timed out: ${err.rawMessage}` : 'Command timed out',
			kind: 'timeout'
		};
	}
	if (
		err instanceof ConnectError &&
		err.code === Code.Unavailable &&
		/^HTTP 504\b/i.test(err.rawMessage)
	) {
		return {
			message: 'Command timed out (upstream gateway timeout: HTTP 504)',
			kind: 'timeout'
		};
	}
	return {
		message: err instanceof Error ? err.message : String(err),
		kind: 'error'
	};
}

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
	/** Categorizes an error result for UI presentation. */
	errorKind?: ErrorKind;
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
	/** Pagination token for subsequent BGPPeerRoutes pages. */
	nextPageToken: string;
	/** True while fetching an additional page of peer routes. */
	loadingMore: boolean;
	/** Current retry attempt count (>0 when retrying after ResourceExhausted). */
	retryAttempt: number;
}

export interface LastRunMeta {
	command: CommandValue;
	parameter: string;
	routerOrder: string[];
	routers: Pb.Router[];
}

export const command = writable<CommandValue | ''>('');
export const parameter = writable<string>('');
/** Last submission's results, keyed by routerId.toString(). */
export const results = writable<Record<string, ExecResult>>({});
/** Metadata describing the most recent run, decoupled from subsequent picker edits. */
export const lastRun = writable<LastRunMeta | null>(null);
/** Whether at least one submission has happened in this session. */
export const hasRun = writable<boolean>(false);
/** Whether a submission is currently in flight. */
export const isRunning = writable<boolean>(false);

let activeRunId = 0;
let activeController: AbortController | null = null;

/** Parses a `bgp_peer_routes` parameter string into structured fields. */
export function parsePeerRoutesParam(param: string): {
	peerIp: string;
	queryType: PeerRouteQueryTypeName;
	peerName: string;
} {
	const parts = param.trim().split(/\s+/).filter(Boolean);
	if (parts.length === 0) {
		return { peerIp: '', queryType: 'received', peerName: '' };
	}

	let peerIp = '';
	let queryType: PeerRouteQueryTypeName = 'received';
	let peerName = '';

	const first = parts[0];
	let idx = 1;
	if (first.startsWith('name=')) {
		peerName = first.slice('name='.length);
	} else {
		peerIp = first;
	}

	if (parts[idx] && PEER_ROUTE_TYPES.has(parts[idx].toLowerCase())) {
		queryType = parts[idx].toLowerCase() as PeerRouteQueryTypeName;
		idx++;
	} else if (parts[idx] && !peerName) {
		peerName = parts.slice(idx).join(' ');
		idx = parts.length;
	}

	if (idx < parts.length && !peerName) {
		peerName = parts.slice(idx).join(' ');
	}

	return { peerIp, queryType, peerName };
}

/** Formats structured `bgp_peer_routes` fields into a canonical parameter string. */
export function formatPeerRoutesParam(
	peerIp: string,
	queryType: PeerRouteQueryTypeName,
	peerName: string
): string {
	const ip = peerIp.trim();
	const name = peerName.trim();
	if (!ip && !name) return '';
	if (!ip && name) {
		return `name=${name} ${queryType}`;
	}
	return name ? `${ip} ${queryType} ${name}` : `${ip} ${queryType}`;
}

function toPeerRouteQueryType(typeStr: PeerRouteQueryTypeName): Pb.PeerRouteQueryType {
	switch (typeStr) {
		case 'received':
			return Pb.PeerRouteQueryType.RECEIVED;
		case 'accepted':
			return Pb.PeerRouteQueryType.ACCEPTED;
		case 'rejected':
			return Pb.PeerRouteQueryType.REJECTED;
		case 'advertised':
			return Pb.PeerRouteQueryType.ADVERTISED;
	}
}

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
		| Pb.BGPPeerRoutesResponse
): ParsedPayload | null {
	const ok = (res as { parseStatus?: Pb.ParseStatus }).parseStatus === Pb.ParseStatus.OK;
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
		case 'bgp_aspath_regex':
		case 'bgp_peer_routes': {
			const d = (
				res as
					| Pb.BGPRouteResponse
					| Pb.BGPCommunityResponse
					| Pb.BGPLargeCommunityResponse
					| Pb.BGPASPathResponse
					| Pb.BGPPeerRoutesResponse
			).parsed;
			return d ? { kind: 'bgp_paths', data: d } : null;
		}
	}
}

const MAX_RETRIES = 5;

function sleepWithAbort(ms: number, signal?: AbortSignal): Promise<void> {
	if (signal?.aborted) {
		return Promise.reject(new DOMException('Aborted', 'AbortError'));
	}
	return new Promise((resolve, reject) => {
		const onAbort = () => {
			clearTimeout(timer);
			reject(new DOMException('Aborted', 'AbortError'));
		};
		const timer = setTimeout(() => {
			signal?.removeEventListener('abort', onAbort);
			resolve();
		}, ms);
		signal?.addEventListener('abort', onAbort, { once: true });
	});
}

async function callWithRetry<T>(
	fn: () => Promise<T>,
	signal?: AbortSignal,
	onRetry?: (attempt: number) => void
): Promise<T> {
	let backoff = 100;
	const maxBackoff = 5000;
	let attempt = 0;

	while (true) {
		if (signal?.aborted) {
			throw new DOMException('Aborted', 'AbortError');
		}
		try {
			return await fn();
		} catch (err) {
			if (signal?.aborted) {
				throw new DOMException('Aborted', 'AbortError');
			}
			if (
				err instanceof ConnectError &&
				err.code === Code.ResourceExhausted &&
				attempt < MAX_RETRIES
			) {
				attempt++;
				onRetry?.(attempt);
				const jitter = Math.random() * backoff * 0.3;
				const waitMs = Math.min(maxBackoff, Math.round(backoff + jitter));
				console.warn(`Resource exhausted, retry ${attempt}/${MAX_RETRIES} in ${waitMs}ms...`);
				await sleepWithAbort(waitMs, signal);
				backoff = Math.min(maxBackoff, backoff * 2);
				continue;
			}
			throw err;
		}
	}
}

function makeSeedResult(router: Pb.Router, cmd: CommandValue, param: string): ExecResult {
	return {
		routerId: router.id.toString(),
		routerName: router.name,
		routerLocation: router.location ?? '',
		status: 'pending',
		timestamp: null,
		bytes: null,
		error: null,
		queryCommand: cmd,
		queryParameter: param,
		resolvedCommand: resolveCommand(cmd, param),
		parsed: null,
		parserKind: Pb.ParserKind.UNSPECIFIED,
		parseStatus: Pb.ParseStatus.DISABLED,
		cached: false,
		nextPageToken: '',
		loadingMore: false,
		retryAttempt: 0
	};
}

async function runOne(
	runId: number,
	router: Pb.Router,
	cmd: CommandValue,
	param: string,
	signal: AbortSignal
): Promise<ExecResult> {
	const key = router.id.toString();
	const base: ExecResult = {
		...makeSeedResult(router, cmd, param),
		status: 'running'
	};
	if (runId === activeRunId) {
		results.update((r) => ({ ...r, [key]: base }));
	}

	const client = LookingGlassClient();
	let isCached = false;
	const callOptions = {
		signal,
		onHeader: (headers: Headers) => {
			if (headers.get('x-cache') === 'HIT') {
				isCached = true;
			}
		}
	};

	try {
		const res = await callWithRetry(
			async () => {
				switch (cmd) {
					case 'ping':
						return await client.ping({ routerId: router.id, target: param }, callOptions);
					case 'traceroute':
						return await client.traceroute({ routerId: router.id, target: param }, callOptions);
					case 'bgp_summary':
						return await client.bGPSummary({ routerId: router.id }, callOptions);
					case 'bgp_route':
						return await client.bGPRoute({ routerId: router.id, target: param }, callOptions);
					case 'bgp_community': {
						const parts = param.split(':');
						if (parts.length === 2) {
							return await client.bGPCommunity(
								{
									routerId: router.id,
									community: { asn: parseInt(parts[0], 10), value: parseInt(parts[1], 10) }
								},
								callOptions
							);
						} else if (parts.length === 3) {
							return await client.bGPLargeCommunity(
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
					}
					case 'bgp_aspath_regex':
						return await client.bGPASPath({ routerId: router.id, pattern: param }, callOptions);
					case 'bgp_peer_routes': {
						const { peerIp, queryType, peerName } = parsePeerRoutesParam(param);
						return await client.bGPPeerRoutes(
							{
								routerId: router.id,
								peerIp,
								peerName,
								queryType: toPeerRouteQueryType(queryType)
							},
							callOptions
						);
					}
					default:
						throw new Error(`Unknown command: ${cmd}`);
				}
			},
			signal,
			(attempt) => {
				if (runId !== activeRunId) return;
				results.update((r) => {
					const cur = r[key];
					if (!cur) return r;
					return { ...r, [key]: { ...cur, retryAttempt: attempt } };
				});
			}
		);

		const tsSeconds = res.timestamp?.seconds ? Number(res.timestamp.seconds) : Date.now() / 1000;
		const nextPageToken =
			cmd === 'bgp_peer_routes' ? ((res as Pb.BGPPeerRoutesResponse).nextPageToken ?? '') : '';
		const done: ExecResult = {
			...base,
			status: 'done',
			bytes: res.result,
			timestamp: new Date(tsSeconds * 1000),
			parsed: pickParsed(cmd, res),
			parserKind: (res as { parserKind?: Pb.ParserKind }).parserKind ?? Pb.ParserKind.UNSPECIFIED,
			parseStatus: (res as { parseStatus?: Pb.ParseStatus }).parseStatus ?? Pb.ParseStatus.DISABLED,
			cached: isCached,
			nextPageToken,
			retryAttempt: 0
		};
		if (runId === activeRunId) {
			results.update((r) => ({ ...r, [key]: done }));
		}
		return done;
	} catch (err) {
		const { message, kind } = formatRpcError(err, signal.aborted);
		const failed: ExecResult = {
			...base,
			status: 'error',
			error: message,
			errorKind: kind,
			timestamp: new Date(),
			retryAttempt: 0
		};
		if (runId === activeRunId) {
			results.update((r) => ({ ...r, [key]: failed }));
		}
		if (kind !== 'canceled') {
			console.error(`[${router.name}] ${cmd} failed:`, err);
		}
		return failed;
	}
}

/** Cancels the currently active query execution, if any. */
export function cancelRun() {
	if (activeController) {
		activeController.abort();
		activeController = null;
	}
	isRunning.set(false);
}

/** Executes cmd and param against an explicit list of routers. */
export async function runWithRouters(cmd: CommandValue, param: string, routers: Pb.Router[]) {
	if (routers.length === 0 || !cmd) return;
	if (!COMMANDS_NO_PARAM.includes(cmd) && !param) return;

	if (activeController) {
		activeController.abort();
	}
	const controller = new AbortController();
	activeController = controller;
	const runId = ++activeRunId;

	hasRun.set(true);
	isRunning.set(true);
	lastRun.set({
		command: cmd,
		parameter: param,
		routerOrder: routers.map((r) => r.id.toString()),
		routers
	});
	onRunStarted();

	const seed: Record<string, ExecResult> = {};
	for (const r of routers) {
		seed[r.id.toString()] = makeSeedResult(r, cmd, param);
	}
	results.set(seed);

	await Promise.all(routers.map((r) => runOne(runId, r, cmd, param, controller.signal)));

	if (runId === activeRunId) {
		isRunning.set(false);
		activeController = null;
		pushHistory(cmd, param, routers, get(results));
	}
}

/** Submits the current command and parameter against the selected routers. */
export async function run(cmd: CommandValue, param: string) {
	const routers = get(selectedRouters);
	return runWithRouters(cmd, param, routers);
}

/** Re-executes the last run against the same routers. */
export async function rerunLast() {
	const prev = get(lastRun);
	if (!prev) return;
	return runWithRouters(prev.command, prev.parameter, prev.routers);
}

/** Fetches the next page of `bgp_peer_routes` for the given router and appends its paths. */
export async function loadMorePeerRoutes(routerId: string) {
	const current = get(results)[routerId];
	if (
		!current ||
		current.queryCommand !== 'bgp_peer_routes' ||
		!current.nextPageToken ||
		current.loadingMore
	) {
		return;
	}

	results.update((r) => {
		const cur = r[routerId];
		if (!cur) return r;
		return { ...r, [routerId]: { ...cur, loadingMore: true } };
	});

	try {
		const client = LookingGlassClient();
		const { peerIp, queryType, peerName } = parsePeerRoutesParam(current.queryParameter);
		const res = await client.bGPPeerRoutes({
			routerId: BigInt(current.routerId),
			peerIp,
			peerName,
			queryType: toPeerRouteQueryType(queryType),
			pageToken: current.nextPageToken
		});

		results.update((r) => {
			const cur = r[routerId];
			if (!cur) return r;

			const prevPaths = cur.parsed?.kind === 'bgp_paths' ? (cur.parsed.data.paths ?? []) : [];
			const nextPaths = res.parsed?.paths ?? [];
			const mergedParsed: ParsedPayload | null =
				cur.parsed?.kind === 'bgp_paths'
					? {
							kind: 'bgp_paths',
							data: {
								...cur.parsed.data,
								paths: [...prevPaths, ...nextPaths]
							}
						}
					: pickParsed('bgp_peer_routes', res);

			let mergedBytes = cur.bytes;
			if (res.result && res.result.byteLength > 0) {
				if (!cur.bytes || cur.bytes.byteLength === 0) {
					mergedBytes = res.result;
				} else {
					const combined = new Uint8Array(cur.bytes.byteLength + 1 + res.result.byteLength);
					combined.set(cur.bytes, 0);
					combined[cur.bytes.byteLength] = 10;
					combined.set(res.result, cur.bytes.byteLength + 1);
					mergedBytes = combined;
				}
			}

			return {
				...r,
				[routerId]: {
					...cur,
					bytes: mergedBytes,
					parsed: mergedParsed,
					nextPageToken: res.nextPageToken ?? '',
					loadingMore: false
				}
			};
		});
	} catch (err) {
		const { message, kind } = formatRpcError(err);
		pushToast({
			kind: 'error',
			title: kind === 'timeout' ? 'Command timed out' : 'Could not load more routes',
			message
		});
		results.update((r) => {
			const cur = r[routerId];
			if (!cur) return r;
			return { ...r, [routerId]: { ...cur, loadingMore: false } };
		});
	}
}

/** Restores a past history entry into the command dock and results sheet. */
export function restoreHistoryEntry(entry: HistoryEntry) {
	cancelRun();
	activeRunId++;
	command.set(entry.command);
	parameter.set(entry.parameter);
	const restored: Record<string, ExecResult> = {};
	for (const [k, v] of Object.entries(entry.results)) {
		restored[k] = {
			...v,
			nextPageToken: v.nextPageToken ?? '',
			loadingMore: false,
			retryAttempt: 0
		};
	}
	results.set(restored);
	lastRun.set({
		command: entry.command,
		parameter: entry.parameter,
		routerOrder: entry.routers.map((r) => r.id.toString()),
		routers: entry.routers
	});
	hasRun.set(true);
	onRunStarted();
}

/** Pre-fills the command dock from an interactive table pivot action. */
export function pivotQuery(opts: {
	command: CommandValue;
	parameter: string;
	selectRouterId?: string;
}) {
	command.set(opts.command);
	parameter.set(opts.parameter);
	if (opts.selectRouterId) {
		const targetId = BigInt(opts.selectRouterId);
		const exists = get(catalogueRouters).some(
			(r) => r.id === targetId && r.health?.healthy === true
		);
		if (exists) {
			clearSelection();
			selectMany([targetId]);
		}
	}
	pushToast({
		kind: 'info',
		title: 'Query ready',
		message: `${opts.command} ${opts.parameter}`.trim(),
		ttl: 2500
	});
}

/** Resets query stores; for unit tests. */
export function _resetQuery() {
	cancelRun();
	activeRunId = 0;
	command.set('');
	parameter.set('');
	results.set({});
	lastRun.set(null);
	hasRun.set(false);
	isRunning.set(false);
}
