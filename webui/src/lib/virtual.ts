export interface VirtualWindow {
	virtualized: boolean;
	startIdx: number;
	endIdx: number;
	padTopPx: number;
	padBotPx: number;
}

export interface VirtualWindowOptions {
	rowPx?: number;
	threshold?: number;
	overscan?: number;
}

/** Computes visible row slice and spacer heights for a virtualized table. */
export function computeVirtualWindow(
	total: number,
	scrollTop: number,
	viewHeight: number,
	opts: VirtualWindowOptions = {}
): VirtualWindow {
	const rowPx = opts.rowPx ?? 28;
	const threshold = opts.threshold ?? 200;
	const overscan = opts.overscan ?? 6;
	const virtualized = total > threshold;
	if (!virtualized) {
		return {
			virtualized: false,
			startIdx: 0,
			endIdx: total,
			padTopPx: 0,
			padBotPx: 0
		};
	}
	const startIdx = Math.max(0, Math.floor(Math.max(0, scrollTop) / rowPx) - overscan);
	const visibleCount = Math.ceil(Math.max(rowPx, viewHeight) / rowPx) + 2 * overscan;
	const endIdx = Math.min(total, startIdx + visibleCount);
	return {
		virtualized: true,
		startIdx,
		endIdx,
		padTopPx: startIdx * rowPx,
		padBotPx: Math.max(0, (total - endIdx) * rowPx)
	};
}
