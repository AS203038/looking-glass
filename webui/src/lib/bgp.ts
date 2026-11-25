/** Maps a BGP session state string to its semantic badge CSS class. */
export function bgpStateBadgeClass(state: string): string {
	const token = state.trim().toLowerCase().split(/[\s(]/)[0];
	switch (token) {
		case 'established':
			return 'lg-badge-success';
		case 'idle':
		case 'connect':
			return 'lg-badge-warning';
		case 'active':
		case 'opensent':
		case 'openconfirm':
			return 'lg-badge-info';
		default:
			return 'lg-badge-danger';
	}
}

/** Formats a BGP session or route age in seconds into a compact human string. */
export function formatBgpAge(seconds: bigint | number): string {
	const s = Number(seconds);
	if (!Number.isFinite(s) || s <= 0) return '—';
	if (s < 60) return `${Math.floor(s)}s`;
	const d = Math.floor(s / 86400);
	const h = Math.floor((s % 86400) / 3600);
	const m = Math.floor((s % 3600) / 60);
	if (d > 0) return `${d}d${h}h`;
	if (h > 0) return `${h}h${m}m`;
	return `${m}m`;
}
