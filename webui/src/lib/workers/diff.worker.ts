import { diffLines } from 'diff';

export interface DiffWorkerRequest {
	jobId: number;
	oldStr: string;
	newStr: string;
}

export interface DiffWorkerLine {
	value: string;
	added?: boolean;
	removed?: boolean;
}

self.onmessage = (e: MessageEvent<DiffWorkerRequest>) => {
	const { jobId, oldStr, newStr } = e.data;
	try {
		const diffs = diffLines(oldStr, newStr);
		const lines: DiffWorkerLine[] = [];
		for (const part of diffs) {
			if (!part.value) continue;
			const partLines = part.value.replace(/\n+$/, '').split('\n');
			for (const l of partLines) {
				lines.push({ value: l, added: part.added, removed: part.removed });
			}
		}
		self.postMessage({ type: 'success', jobId, lines });
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		self.postMessage({ type: 'error', jobId, error: message });
	}
};
