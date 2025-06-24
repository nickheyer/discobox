<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import type { Route, Service } from '$lib/types';
	
	let routes = $state<Route[]>([]);
	let services = $state<Service[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingRoute = $state<Route | null>(null);
	let deleteConfirmOpen = $state(false);
	let routeToDelete = $state<string | null>(null);
	let formData = $state({
		id: '',
		priority: 100,
		host: '',
		path_prefix: '',
		path_regex: '',
		headers: {} as Record<string, string>,
		service_id: '',
		middlewares: [] as string[]
	});
	
	const availableMiddlewares = [
		'compression',
		'cors',
		'rate-limit',
		'circuit-breaker',
		'security-headers',
		'access-log'
	];
	
	onMount(async () => {
		if (!$isAuthenticated) {
			goto('/login');
			return;
		}
		await Promise.all([loadRoutes(), loadServices()]);
	});
	
	async function loadRoutes() {
		try {
			routes = await api.getRoutes() || [];
		} catch (error) {
			console.error('Failed to load routes:', error);
			routes = [];
		}
	}
	
	async function loadServices() {
		try {
			services = await api.getServices() || [];
			loading = false;
		} catch (error) {
			console.error('Failed to load services:', error);
			services = [];
			loading = false;
		}
	}
	
	function openModal(route?: Route) {
		if (route) {
			editingRoute = route;
			// Deep clone to avoid mutation
			formData = {
				id: route.id,
				priority: route.priority,
				host: route.host || '',
				path_prefix: route.path_prefix || '',
				path_regex: route.path_regex || '',
				headers: route.headers ? { ...route.headers } : {},
				service_id: route.service_id || '',
				middlewares: route.middlewares ? [...route.middlewares] : []
			};
		} else {
			editingRoute = null;
			formData = {
				id: '',
				priority: 100,
				host: '',
				path_prefix: '',
				path_regex: '',
				headers: {},
				service_id: '',
				middlewares: []
			};
		}
		showModal = true;
	}
	
	function closeModal() {
		showModal = false;
		editingRoute = null;
	}
	
	async function saveRoute() {
		try {
			// Build data object with only defined values
			const data: any = {
				id: formData.id,
				priority: formData.priority,
				service_id: formData.service_id,
				middlewares: formData.middlewares
			};
			
			// Add optional fields only if they have values
			if (formData.host) data.host = formData.host;
			if (formData.path_prefix) data.path_prefix = formData.path_prefix;
			if (formData.path_regex) data.path_regex = formData.path_regex;
			if (Object.keys(formData.headers).length > 0) {
				data.headers = formData.headers;
			}
			
			if (editingRoute) {
				await api.updateRoute(editingRoute.id, data);
			} else {
				await api.createRoute(data);
			}
			
			await loadRoutes();
			closeModal();
		} catch (error) {
			console.error('Failed to save route:', error);
			console.error('Failed to save route:', error);
		}
	}
	
	async function deleteRoute(id: string) {
		try {
			await api.deleteRoute(id);
			await loadRoutes();
		} catch (error) {
			console.error('Failed to delete route:', error);
		}
	}
	
	function toggleMiddleware(mw: string) {
		if (formData.middlewares.includes(mw)) {
			formData.middlewares = formData.middlewares.filter(m => m !== mw);
		} else {
			formData.middlewares = [...formData.middlewares, mw];
		}
	}
</script>

{#if $isAuthenticated}
	<Navbar />
	
	<div class="container mx-auto p-4 max-w-7xl">
		<div class="flex justify-between items-center mb-6">
			<h1 class="text-3xl font-bold">Routes</h1>
			<button class="btn btn-primary" onclick={() => openModal()}>
				<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				Add Route
			</button>
		</div>
		
		{#if loading}
			<div class="flex justify-center items-center h-64">
				<span class="loading loading-spinner loading-lg"></span>
			</div>
		{:else}
			<div class="card bg-base-200 shadow-sm">
				<div class="card-body p-0">
				<div class="overflow-x-auto">
					<table class="table table-zebra w-full">
					<thead>
						<tr>
							<th class="w-20">Priority</th>
							<th class="min-w-[150px]">Host</th>
							<th class="min-w-[200px]">Path</th>
							<th class="min-w-[150px]">Service</th>
							<th class="min-w-[200px]">Middlewares</th>
							<th class="w-32">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each [...(routes || [])].sort((a, b) => b.priority - a.priority) as route}
							<tr class="hover">
								<td>
									<span class="badge badge-primary badge-sm">{route.priority}</span>
								</td>
								<td>
									<span class="font-mono text-xs block truncate max-w-[150px]" title={route.host || '*'}>
										{route.host || '*'}
									</span>
								</td>
								<td>
									<span class="font-mono text-xs">
										{#if route.path_prefix}
											<span class="text-info">prefix:</span>
											<span class="block truncate max-w-[200px]" title={route.path_prefix}>{route.path_prefix}</span>
										{:else if route.path_regex}
											<span class="text-warning">regex:</span>
											<span class="block truncate max-w-[200px]" title={route.path_regex}>{route.path_regex}</span>
										{:else}
											<span class="text-base-content/50">*</span>
										{/if}
									</span>
								</td>
								<td>
									<span class="text-sm block truncate max-w-[150px]" title={services.find(s => s.id === route.service_id)?.name || route.service_id}>
										{services.find(s => s.id === route.service_id)?.name || route.service_id}
									</span>
								</td>
								<td>
									<div class="flex flex-wrap gap-1 max-w-[200px]">
										{#each route.middlewares || [] as mw}
											<span class="badge badge-ghost badge-xs">{mw}</span>
										{/each}
									</div>
								</td>
								<td>
									<div class="flex gap-1">
										<button class="btn btn-xs btn-ghost" onclick={() => openModal(route)}>Edit</button>
										<button class="btn btn-xs btn-ghost btn-error" onclick={() => deleteRoute(route.id)}>Delete</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
					</table>
				</div>
				</div>
				
				{#if !routes || routes.length === 0}
					<div class="text-center py-12">
						<p class="text-base-content/70 mb-4">No routes configured yet.</p>
						<button class="btn btn-primary" onclick={() => openModal()}>Add Your First Route</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
	
	<!-- Modal -->
	<dialog class="modal" class:modal-open={showModal}>
		<div class="modal-box max-w-3xl">
			<h3 class="text-xl font-bold mb-6">
				{editingRoute ? 'Edit Route' : 'Add Route'}
			</h3>
			
			<form class="space-y-6">
				<!-- Basic Information -->
				<div class="space-y-4">
					<div class="divider text-sm">Basic Information</div>
					
					<div class="grid grid-cols-1 md:grid-cols-3 gap-4">
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Route ID</legend>
							<input
								type="text"
								class="input"
								bind:value={formData.id}
								placeholder="my-route"
								disabled={!!editingRoute}
							/>
							<p class="label">Unique identifier</p>
						</fieldset>
						
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Priority</legend>
							<input
								type="number"
								class="input"
								bind:value={formData.priority}
								min="1"
								max="10000"
								required
								placeholder="1-10000"
							/>
							<p class="label">Higher number = higher priority</p>
						</fieldset>
						
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Target Service</legend>
							<select
								class="select"
								bind:value={formData.service_id}
								required
							>
								<option value="" disabled>Select a service</option>
								{#each services as service}
									<option value={service.id}>
										{service.name} ({service.id})
									</option>
								{/each}
							</select>
							<p class="label">Required</p>
						</fieldset>
					</div>
				</div>
				
				<!-- Matching Rules -->
				<div class="space-y-4">
					<div class="divider text-sm">Matching Rules</div>
					<p class="text-sm text-base-content/70">At least one matching rule is required</p>
					
					<label class="input">
						Host
						<input
							type="text"
							class="grow"
							bind:value={formData.host}
							placeholder="example.com or *.example.com"
						/>
						<span class="badge badge-neutral badge-xs">Optional</span>
					</label>
					
					<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
						<label class="input">
							Path Prefix
							<input
								type="text"
								class="grow"
								bind:value={formData.path_prefix}
								placeholder="/api"
							/>
							<span class="badge badge-neutral badge-xs">Optional</span>
						</label>
						
						<label class="input">
							Path Regex
							<input
								type="text"
								class="grow"
								bind:value={formData.path_regex}
								placeholder="^/api/v[0-9]+/.*"
							/>
							<span class="badge badge-neutral badge-xs">Optional</span>
						</label>
					</div>
				</div>
				
				<!-- Middlewares -->
				<div class="space-y-4">
					<div class="divider text-sm">Middlewares</div>
					
					<div class="grid grid-cols-2 md:grid-cols-3 gap-3">
						{#each availableMiddlewares as mw}
							<label class="label cursor-pointer bg-base-200 rounded-lg p-3 hover:bg-base-300 transition-colors">
								<span class="label-text">{mw}</span>
								<input
									type="checkbox"
									class="checkbox checkbox-primary"
									checked={formData.middlewares.includes(mw)}
									onchange={() => toggleMiddleware(mw)}
								/>
							</label>
						{/each}
					</div>
				</div>
			</form>
			
			<div class="modal-action">
				<button class="btn btn-ghost" onclick={closeModal}>Cancel</button>
				<button class="btn btn-primary" onclick={saveRoute}>Save Route</button>
			</div>
		</div>
		<form method="dialog" class="modal-backdrop">
			<button onclick={closeModal}>close</button>
		</form>
	</dialog>
{/if}