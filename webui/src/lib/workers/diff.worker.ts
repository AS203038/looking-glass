import { diffLines } from 'diff';

self.onmessage = (e: MessageEvent<{ oldStr: string; newStr: string }>) => {
	const { oldStr, newStr } = e.data;
	try {
		const diffs = diffLines(oldStr, newStr);
		const lines: { value: string; added?: boolean; removed?: boolean }[] = [];
		for (const part of diffs) {
			const partLines = part.value.replace(/\n+$/, '').split('\n');
			for (const l of partLines) {
				lines.push({ value: l, added: part.added, removed: part.removed });
			}
		}
		self.postMessage({ type: 'success', lines });
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		self.postMessage({ type: 'error', error: message });
	}
};
