// @vitest-environment node
import { ESLint } from 'eslint';
import { describe, expect, it } from 'vitest';

const eslint = new ESLint({ cwd: import.meta.dirname });

async function rttRuleHits(mustache: string): Promise<number> {
	const code = `<script lang="ts">\n\tlet { p, hasReply }: { p: { rttMs: number }; hasReply: boolean } = $props();\n\tconst float32 = (v: number) => String(v);\n</script>\n\n<span>${mustache}</span>\n`;
	const [result] = await eslint.lintText(code, {
		filePath: 'src/lib/components/views/DiffTracerouteView.svelte'
	});
	return result.messages.filter((m) => m.ruleId === 'no-restricted-syntax').length;
}

describe('the *Ms display rule', () => {
	it.each([
		'{p.rttMs}',
		"{p.rttMs + ' ms'}",
		'{`${p.rttMs} ms`}',
		"{hasReply ? p.rttMs : '*'}",
		"{hasReply ? '*' : p.rttMs}"
	])('rejects %s', async (mustache) => {
		expect(await rttRuleHits(mustache)).toBe(1);
	});

	it.each([
		"{float32(p.rttMs) + ' ms'}",
		"{p.rttMs > 0 ? float32(p.rttMs) + ' ms' : '*'}",
		"{p.rttMs ? float32(p.rttMs) + ' ms' : '*'}"
	])('accepts %s', async (mustache) => {
		expect(await rttRuleHits(mustache)).toBe(0);
	});
});
