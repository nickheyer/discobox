<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	
	let routes = $state<any[]>([]);
	let services = $state<any[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingRoute = $state<any>(null);
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
			routes = await api.getRoutes();
		} catch (error) {
			console.error('Failed to load routes:', error);
		}
	}
	
	async function loadServices() {
		try {
			services = await api.getServices();
			loading = false;
		} catch (error) {
			console.error('Failed to load services:', error);
			loading = false;
		}
	}
	
	function openModal(route?: any) {
		if (route) {
			editingRoute = route;
			formData = {
				...route,
				headers: { ...(route.headers || {}) },
				middlewares: [...(route.middlewares || [])]
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
			const data = { ...formData };
			// Remove empty values
			if (!data.host) delete data.host;
			if (!data.path_prefix) delete data.path_prefix;
			if (!data.path_regex) delete data.path_regex;
			if (Object.keys(data.headers).length === 0) delete data.headers;
			
			if (editingRoute) {
				await api.updateRoute(editingRoute.id, data);
			} else {
				await api.createRoute(data);
			}
			
			await loadRoutes();
			closeModal();
		} catch (error) {
			console.error('Failed to save route:', error);
			alert('Failed to save route');
		}
	}
	
	async function deleteRoute(id: string) {
		if (!confirm('Are you sure you want to delete this route?')) return;
		
		try {
			await api.deleteRoute(id);
			await loadRoutes();
		} catch (error) {
			console.error('Failed to delete route:', error);
			alert('Failed to delete route');
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
	
	<div class="container mx-auto p-4">
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
			<div class="overflow-x-auto">
				<table class="table">
					<thead>
						<tr>
							<th>Priority</th>
							<th>Host</th>
							<th>Path</th>
							<th>Service</th>
							<th>Middlewares</th>
							<th>Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each routes.sort((a, b) => b.priority - a.priority) as route}
							<tr>
								<td>
									<span class="badge badge-primary">{route.priority}</span>
								</td>
								<td class="font-mono text-sm">
									{route.host || '*'}
								</td>
								<td class="font-mono text-sm">
									{#if route.path_prefix}
										<span class="text-info">prefix:</span> {route.path_prefix}
									{:else if route.path_regex}
										<span class="text-warning">regex:</span> {route.path_regex}
									{:else}
										*
									{/if}
								</td>
								<td>
									{services.find(s => s.id === route.service_id)?.name || route.service_id}
								</td>
								<td>
									<div class="flex flex-wrap gap-1">
										{#each route.middlewares || [] as mw}
											<span class="badge badge-sm">{mw}</span>
										{/each}
									</div>
								</td>
								<td>
									<div class="flex gap-2">
										<button class="btn btn-xs" onclick={() => openModal(route)}>Edit</button>
										<button class="btn btn-xs btn-error" onclick={() => deleteRoute(route.id)}>Delete</button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
				
				{#if routes.length === 0}
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
		<div class="modal-box max-w-2xl">
			<h3 class="font-bold text-lg mb-4">
				{editingRoute ? 'Edit Route' : 'Add Route'}
			</h3>
			
			<div class="space-y-4">
				<div class="grid grid-cols-2 gap-4">
					<div class="form-control">
						<label for="route-id" class="label">
							<span class="label-text">Route ID</span>
						</label>
						<input
							id="route-id"
							type="text"
							class="input input-bordered"
							bind:value={formData.id}
							placeholder="my-route"
							disabled={!!editingRoute}
						/>
					</div>
					
					<div class="form-control">
						<label for="priority" class="label">
							<span class="label-text">Priority</span>
						</label>
						<input
							id="priority"
							type="number"
							class="input input-bordered"
							bind:value={formData.priority}
							min="1"
						/>
					</div>
				</div>
				
				<div class="form-control">
					<label for="service" class="label">
						<span class="label-text">Service</span>
					</label>
					<select
						id="service"
						class="select select-bordered"
						bind:value={formData.service_id}
						required
					>
						<option value="">Select a service</option>
						{#each services as service}
							<option value={service.id}>{service.name} ({service.id})</option>
						{/each}
					</select>
				</div>
				
				<div class="form-control">
					<label for="host" class="label">
						<span class="label-text">Host (optional)</span>
					</label>
					<input
						id="host"
						type="text"
						class="input input-bordered"
						bind:value={formData.host}
						placeholder="example.com or *.example.com"
					/>
				</div>
				
				<div class="grid grid-cols-2 gap-4">
					<div class="form-control">
						<label for="path-prefix" class="label">
							<span class="label-text">Path Prefix (optional)</span>
						</label>
						<input
							id="path-prefix"
							type="text"
							class="input input-bordered"
							bind:value={formData.path_prefix}
							placeholder="/api"
						/>
					</div>
					
					<div class="form-control">
						<label for="path-regex" class="label">
							<span class="label-text">Path Regex (optional)</span>
						</label>
						<input
							id="path-regex"
							type="text"
							class="input input-bordered"
							bind:value={formData.path_regex}
							placeholder="^/api/v[0-9]+/.*"
						/>
					</div>
				</div>
				
				<div class="form-control">
					<label class="label">
						<span class="label-text">Middlewares</span>
					</label>
					<div class="flex flex-wrap gap-2">
						{#each availableMiddlewares as mw}
							<label class="label cursor-pointer gap-2">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									checked={formData.middlewares.includes(mw)}
									onchange={() => toggleMiddleware(mw)}
								/>
								<span class="label-text">{mw}</span>
							</label>
						{/each}
					</div>
				</div>
			</div>
			
			<div class="modal-action">
				<button class="btn" onclick={closeModal}>Cancel</button>
				<button class="btn btn-primary" onclick={saveRoute}>Save</button>
			</div>
		</div>
		<form method="dialog" class="modal-backdrop">
			<button onclick={closeModal}>close</button>
		</form>
	</dialog>
{/if}