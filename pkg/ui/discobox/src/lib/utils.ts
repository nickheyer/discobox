export function formatNumber(num: number, decimals: number = 2): string {
	if (num === null || num === undefined) return '0';
	if (num < 1000) return num.toFixed(decimals);
	if (num < 1000000) return (num / 1000).toFixed(decimals) + 'k';
	if (num < 1000000000) return (num / 1000000).toFixed(decimals) + 'M';
	return (num / 1000000000).toFixed(decimals) + 'B';
}

export function formatBytes(bytes: number): string {
	if (bytes === null || bytes === undefined) return '0 B';
	if (bytes < 1024) return bytes + ' B';
	if (bytes < 1048576) return (bytes / 1024).toFixed(2) + ' KB';
	if (bytes < 1073741824) return (bytes / 1048576).toFixed(2) + ' MB';
	return (bytes / 1073741824).toFixed(2) + ' GB';
}

export function formatMemoryMB(mb: number): string {
	if (mb === null || mb === undefined) return '0 MB';
	if (mb < 1024) return mb.toFixed(0) + ' MB';
	return (mb / 1024).toFixed(2) + ' GB';
}

export function formatPercentage(value: number, decimals: number = 2): string {
	if (value === null || value === undefined) return '0%';
	// If value is already a percentage (> 1), don't multiply by 100
	const percentage = value > 1 ? value : value * 100;
	return percentage.toFixed(decimals) + '%';
}

export function formatDuration(ms: number): string {
	if (ms === null || ms === undefined) return '0ms';
	if (ms < 1) return ms.toFixed(3) + 'ms';
	if (ms < 1000) return ms.toFixed(1) + 'ms';
	if (ms < 60000) return (ms / 1000).toFixed(2) + 's';
	return (ms / 60000).toFixed(2) + 'm';
}