<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import type { Health, Metrics, Service, Route } from '$lib/types';
	
	let health = $state<Health | null>(null);
	let metrics = $state<Metrics | null>(null);
	let services = $state<Service[]>([]);
	let routes = $state<Route[]>([]);
	let loading = $state(true);
	
	onMount(async () => {
		if (!$isAuthenticated) {
			goto('/login');
			return;
		}
		
		try {
			const [h, m, s, r] = await Promise.all([
				api.getHealth(),
				api.getMetrics(),
				api.getServices(),
				api.getRoutes()
			]);
			
			health = h;
			metrics = m;
			services = s;
			routes = r;
		} catch (error) {
			console.error('Failed to load data:', error);
		} finally {
			loading = false;
		}
	});
</script>

{#if $isAuthenticated}
	<Navbar />
	
	<div class="container mx-auto p-4">
		{#if loading}
			<div class="flex justify-center items-center h-64">
				<span class="loading loading-spinner loading-lg"></span>
			</div>
		{:else}
			<div class="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 mb-8">
				<div class="stat bg-base-200 rounded-box shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="stat-figure text-primary">
						<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path>
						</svg>
					</div>
					<div class="stat-title">Status</div>
					<div class="stat-value text-primary">{health?.status || 'Unknown'}</div>
					<div class="stat-desc">System health</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="stat-figure text-secondary">
						<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path>
						</svg>
					</div>
					<div class="stat-title">Requests</div>
					<div class="stat-value text-secondary">{metrics?.requests?.per_second || 0}/s</div>
					<div class="stat-desc">{metrics?.requests?.total || 0} total</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="stat-figure text-accent">
						<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"></path>
						</svg>
					</div>
					<div class="stat-title">Services</div>
					<div class="stat-value text-accent">{services.length}</div>
					<div class="stat-desc">{services.filter(s => s.active).length} active</div>
				</div>
				
				<div class="stat bg-base-200 rounded-box shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="stat-figure text-info">
						<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7"></path>
						</svg>
					</div>
					<div class="stat-title">Routes</div>
					<div class="stat-value text-info">{routes.length}</div>
					<div class="stat-desc">Configured routes</div>
				</div>
			</div>
			
			<div class="grid gap-4 grid-cols-1 lg:grid-cols-2">
				<div class="card bg-base-200 shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="card-body">
						<h2 class="card-title">Recent Services</h2>
						<div class="overflow-x-auto">
							<table class="table table-sm w-full">
								<thead>
									<tr>
										<th>Name</th>
										<th>Endpoints</th>
										<th>Status</th>
									</tr>
								</thead>
								<tbody>
									{#each services.slice(0, 5) as service}
										<tr>
											<td class="truncate max-w-[150px]" title={service.name}>{service.name}</td>
											<td>{service.endpoints?.length || 0}</td>
											<td>
												<span class="badge badge-sm" class:badge-success={service.active} class:badge-error={!service.active}>
													{service.active ? 'Active' : 'Inactive'}
												</span>
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
						<div class="card-actions justify-end">
							<a href="/services" class="btn btn-primary btn-sm gap-2">
								<span>View All</span>
								<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
								</svg>
							</a>
						</div>
					</div>
				</div>
				
				<div class="card bg-base-200 shadow-sm hover:shadow-md transition-shadow duration-200">
					<div class="card-body">
						<h2 class="card-title">System Info</h2>
						<div class="space-y-3">
							<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
								<span class="text-base-content/70">Version</span>
								<span class="font-mono">{health?.version || 'Unknown'}</span>
							</div>
							<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
								<span class="text-base-content/70">Uptime</span>
								<span class="font-mono">{metrics?.uptime || 'Unknown'}</span>
							</div>
							<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
								<span class="text-base-content/70">Memory</span>
								<span class="font-mono">{metrics?.system?.memory_mb || 0} MB</span>
							</div>
							<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
								<span class="text-base-content/70">Goroutines</span>
								<span class="font-mono">{metrics?.system?.goroutines || 0}</span>
							</div>
							<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
								<span class="text-base-content/70">Connections</span>
								<span class="font-mono">{metrics?.system?.connections || 0}</span>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
{/if}
