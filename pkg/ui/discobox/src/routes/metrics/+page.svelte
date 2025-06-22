<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	
	let metrics = $state<any>(null);
	let loading = $state(true);
	let interval = $state(5000);
	
	onMount(async () => {
		if (!$isAuthenticated) {
			goto('/login');
			return;
		}
		
		await loadMetrics();

		// Refresh metrics every 5 seconds
		interval = setInterval(loadMetrics, 5000);
	});
	
	onDestroy(() => {
		if (interval) clearInterval(interval);
	});
	
	async function loadMetrics() {
		try {
			metrics = await api.getMetrics();
			loading = false;
		} catch (error) {
			console.error('Failed to load metrics:', error);
			loading = false;
		}
	}
	
	function formatNumber(num: number): string {
		if (num >= 1000000) return (num / 1000000).toFixed(2) + 'M';
		if (num >= 1000) return (num / 1000).toFixed(2) + 'K';
		return num.toString();
	}
</script>

{#if $isAuthenticated}
	<Navbar />
	
	<div class="container mx-auto p-4">
		<h1 class="text-3xl font-bold mb-6">Metrics</h1>
		
		{#if loading}
			<div class="flex justify-center items-center h-64">
				<span class="loading loading-spinner loading-lg"></span>
			</div>
		{:else if metrics}
			<!-- Overview Stats -->
			<div class="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-8">
				<div class="stat bg-base-200 rounded-box">
					<div class="stat-title">Uptime</div>
					<div class="stat-value text-primary">{metrics.uptime || '0s'}</div>
					<div class="stat-desc">System running time</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box">
					<div class="stat-title">Total Requests</div>
					<div class="stat-value text-secondary">{formatNumber(metrics.requests?.total || 0)}</div>
					<div class="stat-desc">{metrics.requests?.per_second || 0} req/s</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box">
					<div class="stat-title">Error Rate</div>
					<div class="stat-value" class:text-error={metrics.requests?.error_rate > 0.05}>
						{((metrics.requests?.error_rate || 0) * 100).toFixed(2)}%
					</div>
					<div class="stat-desc">{metrics.requests?.errors || 0} total errors</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box">
					<div class="stat-title">Active Connections</div>
					<div class="stat-value text-info">{metrics.system?.connections || 0}</div>
					<div class="stat-desc">Current connections</div>
				</div>
			</div>
			
			<!-- Latency Stats -->
			<div class="card bg-base-200 mb-6">
				<div class="card-body">
					<h2 class="card-title">Latency Distribution</h2>
					<div class="grid grid-cols-2 md:grid-cols-4 gap-4">
						<div class="text-center">
							<div class="text-3xl font-bold text-primary">{metrics.requests?.avg_latency_ms || 0}ms</div>
							<div class="text-sm text-base-content/70">Average</div>
						</div>
						<div class="text-center">
							<div class="text-3xl font-bold">{metrics.requests?.p50_latency_ms || 0}ms</div>
							<div class="text-sm text-base-content/70">P50</div>
						</div>
						<div class="text-center">
							<div class="text-3xl font-bold">{metrics.requests?.p95_latency_ms || 0}ms</div>
							<div class="text-sm text-base-content/70">P95</div>
						</div>
						<div class="text-center">
							<div class="text-3xl font-bold text-warning">{metrics.requests?.p99_latency_ms || 0}ms</div>
							<div class="text-sm text-base-content/70">P99</div>
						</div>
					</div>
				</div>
			</div>
			
			<!-- System Resources -->
			<div class="grid gap-4 lg:grid-cols-2">
				<div class="card bg-base-200">
					<div class="card-body">
						<h2 class="card-title">System Resources</h2>
						<div class="space-y-4">
							<div>
								<div class="flex justify-between mb-1">
									<span class="text-sm">Memory Usage</span>
									<span class="text-sm font-mono">{metrics.system?.memory_mb || 0} MB</span>
								</div>
								<progress 
									class="progress progress-primary" 
									value={metrics.system?.memory_mb || 0} 
									max="1024"
								></progress>
							</div>
							
							<div>
								<div class="flex justify-between mb-1">
									<span class="text-sm">CPU Usage</span>
									<span class="text-sm font-mono">{metrics.system?.cpu_percent || 0}%</span>
								</div>
								<progress 
									class="progress" 
									class:progress-success={metrics.system?.cpu_percent < 50}
									class:progress-warning={metrics.system?.cpu_percent >= 50 && metrics.system?.cpu_percent < 80}
									class:progress-error={metrics.system?.cpu_percent >= 80}
									value={metrics.system?.cpu_percent || 0} 
									max="100"
								></progress>
							</div>
							
							<div class="pt-2">
								<div class="flex justify-between">
									<span class="text-sm text-base-content/70">Goroutines</span>
									<span class="text-sm font-mono">{metrics.system?.goroutines || 0}</span>
								</div>
							</div>
						</div>
					</div>
				</div>
				
				<!-- Per-Service Metrics -->
				<div class="card bg-base-200">
					<div class="card-body">
						<h2 class="card-title">Service Performance</h2>
						<div class="overflow-x-auto">
							<table class="table table-sm">
								<thead>
									<tr>
										<th>Service</th>
										<th>Requests</th>
										<th>Errors</th>
										<th>Avg Latency</th>
										<th>Health</th>
									</tr>
								</thead>
								<tbody>
									{#each Object.entries(metrics.services || {}) as [id, svc]}
										<tr>
											<td class="font-mono text-xs">{id}</td>
											<td>{formatNumber(svc?.requests || 0)}</td>
											<td class:text-error={svc?.errors > 0}>{svc?.errors || 0}</td>
											<td>{svc?.avg_latency_ms || 0}ms</td>
											<td>
												<span 
													class="badge badge-xs"
													class:badge-success={svc?.health_status === 'healthy'}
													class:badge-warning={svc?.health_status === 'degraded'}
													class:badge-error={svc?.health_status === 'unhealthy'}
												>
													{svc?.health_status || 'unknown'}
												</span>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				</div>
			</div>
			
			<!-- Auto-refresh indicator -->
			<div class="text-center mt-4 text-sm text-base-content/50">
				Auto-refreshing every 5 seconds
			</div>
		{/if}
	</div>
{/if}