// @vitest-environment node
import { ESLint } from 'eslint';
import { describe, expect, it } from 'vitest';

const eslint = new ESLint({ cwd: import.meta.dirname });

async function floatRuleHits(mustache: string, field = 'rttMs'): Promise<number> {
	const code = `<script lang="ts">\n\tlet { p, hasReply }: { p: { rttMs: number; lossPct: number }; hasReply: boolean } = $props();\n\tconst float32 = (v: number) => String(v);\n</script>\n\n<span>${mustache}</span>\n`;
	const [result] = await eslint.lintText(code, {
		filePath: 'src/lib/components/views/DiffTracerouteView.svelte'
	});
	void field;
	return result.messages.filter((m) => m.ruleId === 'no-restricted-syntax').length;
}

describe('the *Ms / *Pct display rule', () => {
	it.each([
		'{p.rttMs}',
		"{p.rttMs + ' ms'}",
		'{`${p.rttMs} ms`}',
		"{hasReply ? p.rttMs : '*'}",
		"{hasReply ? '*' : p.rttMs}",
		"{String(hasReply) + (hasReply ? p.rttMs : '*')}",
		'{p.lossPct}',
		"{p.lossPct + '%'}",
		'{`${p.lossPct}%`}'
	])('rejects %s', async (mustache) => {
		expect(await floatRuleHits(mustache)).toBe(1);
	});

	it.each([
		"{float32(p.rttMs) + ' ms'}",
		"{p.rttMs > 0 ? float32(p.rttMs) + ' ms' : '*'}",
		"{p.rttMs ? float32(p.rttMs) + ' ms' : '*'}",
		'{(hasReply ? p.rttMs : 0).toFixed(3)}',
		"{float32(hasReply ? p.rttMs : 0) + ' ms'}",
		"{float32(p.lossPct) + '%'}",
		'{p.lossPct.toFixed(0)}%'
	])('accepts %s', async (mustache) => {
		expect(await floatRuleHits(mustache)).toBe(0);
	});
});
