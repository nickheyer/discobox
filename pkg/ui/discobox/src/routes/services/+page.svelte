<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import type { Service } from '$lib/types';
	
	let services = $state<Service[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingService = $state<Service | null>(null);
	let formData = $state({
		id: '',
		name: '',
		endpoints: [''],
		health_path: '/health',
		weight: 1,
		timeout: '30s',
		active: true
	});
	
	onMount(async () => {
		if (!$isAuthenticated) {
			goto('/login');
			return;
		}
		await loadServices();
	});
	
	async function loadServices() {
		try {
			loading = true;
			services = await api.getServices() || [];
		} catch (error) {
			console.error('Failed to load services:', error);
			services = [];
		} finally {
			loading = false;
		}
	}
	
	function openModal(service?: Service) {
		if (service) {
			editingService = service;
			formData = {
				...service,
				endpoints: [...(service.endpoints || [])]
			};
		} else {
			editingService = null;
			formData = {
				id: '',
				name: '',
				endpoints: [''],
				health_path: '/health',
				weight: 1,
				timeout: '30s',
				active: true
			};
		}
		showModal = true;
	}
	
	function closeModal() {
		showModal = false;
		editingService = null;
	}
	
	function addEndpoint() {
		formData.endpoints = [...formData.endpoints, ''];
	}
	
	function removeEndpoint(index: number) {
		formData.endpoints = formData.endpoints.filter((_, i) => i !== index);
	}
	
	async function saveService() {
		try {
			const data = {
				...formData,
				endpoints: formData.endpoints.filter(e => e.trim())
			};
			
			if (editingService) {
				await api.updateService(editingService.id, data);
			} else {
				await api.createService(data);
			}
			
			await loadServices();
			closeModal();
		} catch (error) {
			console.error('Failed to save service:', error);
			console.error('Failed to save service:', error);
		}
	}
	
	async function deleteService(id: string) {
		try {
			await api.deleteService(id);
			await loadServices();
		} catch (error) {
			console.error('Failed to delete service:', error);
		}
	}
</script>

{#if $isAuthenticated}
	<Navbar />
	
	<div class="container mx-auto p-4 max-w-7xl">
		<div class="flex justify-between items-center mb-6">
			<h1 class="text-3xl font-bold">Services</h1>
			<button class="btn btn-primary" onclick={() => openModal()}>
				<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
				</svg>
				Add Service
			</button>
		</div>
		
		{#if loading}
			<div class="flex justify-center items-center h-64">
				<span class="loading loading-spinner loading-lg"></span>
			</div>
		{:else}
			<div class="grid gap-4">
				{#each services || [] as service}
					<div class="card bg-base-200 shadow-sm hover:shadow-md transition-shadow duration-200">
						<div class="card-body">
							<div class="flex justify-between items-start">
								<div>
									<h2 class="card-title">{service.name}</h2>
									<p class="text-sm text-base-content/70">ID: {service.id}</p>
								</div>
								<div class="flex gap-2">
									<span class="badge" class:badge-success={service.active} class:badge-error={!service.active}>
										{service.active ? 'Active' : 'Inactive'}
									</span>
								</div>
							</div>
							
							<div class="grid gap-4 mt-4">
								<div>
									<p class="font-semibold mb-2">Endpoints:</p>
									<div class="space-y-1">
										{#each service.endpoints || [] as endpoint}
											<div class="badge badge-outline badge-sm font-mono max-w-full">
												<span class="truncate" title={endpoint}>{endpoint}</span>
											</div>
										{/each}
									</div>
								</div>
								
								<div class="grid grid-cols-2 lg:grid-cols-4 gap-4 text-sm">
									<div class="space-y-1">
										<p class="text-xs text-base-content/60 uppercase tracking-wider">Health Path</p>
										<p class="font-mono">{service.health_path}</p>
									</div>
									<div class="space-y-1">
										<p class="text-xs text-base-content/60 uppercase tracking-wider">Weight</p>
										<p class="font-mono">{service.weight}</p>
									</div>
									<div class="space-y-1">
										<p class="text-xs text-base-content/60 uppercase tracking-wider">Timeout</p>
										<p class="font-mono">{service.timeout}</p>
									</div>
									<div class="space-y-1">
										<p class="text-xs text-base-content/60 uppercase tracking-wider">Max Connections</p>
										<p class="font-mono">{service.max_conns || 'Unlimited'}</p>
									</div>
								</div>
							</div>
							
							<div class="card-actions justify-end mt-4">
								<button class="btn btn-sm btn-ghost" onclick={() => openModal(service)}>Edit</button>
								<button class="btn btn-sm btn-ghost btn-error" onclick={() => deleteService(service.id)}>Delete</button>
							</div>
						</div>
					</div>
				{/each}
				
				{#if !services || services.length === 0}
					<div class="text-center py-12">
						<p class="text-base-content/70 mb-4">No services configured yet.</p>
						<button class="btn btn-primary" onclick={() => openModal()}>Add Your First Service</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
	
	<!-- Modal -->
	<dialog class="modal" class:modal-open={showModal}>
		<div class="modal-box max-w-2xl max-h-[90vh] overflow-y-auto">
			<h3 class="font-bold text-lg mb-4">
				{editingService ? 'Edit Service' : 'Add Service'}
			</h3>
			
			<div class="space-y-4">
				<div class="form-control">
					<label class="label" for="service-id">
						<span class="label-text">Service ID</span>
					</label>
					<input
						id="service-id"
						type="text"
						class="input input-bordered"
						bind:value={formData.id}
						placeholder="my-service"
						disabled={!!editingService}
					/>
				</div>
				
				<div class="form-control">
					<label class="label" for="service-name">
						<span class="label-text">Name</span>
					</label>
					<input
						id="service-name"
						type="text"
						class="input input-bordered"
						bind:value={formData.name}
						placeholder="My Service"
					/>
				</div>
				
				<div class="form-control">
					<div class="label">
						<span class="label-text">Endpoints</span>
					</div>
					{#each formData.endpoints as endpoint, i}
						<div class="flex gap-2 mb-2">
							<input
								type="text"
								class="input input-bordered flex-1"
								bind:value={formData.endpoints[i]}
								placeholder="http://localhost:3000"
							/>
							<button
								class="btn btn-square btn-error"
								onclick={() => removeEndpoint(i)}
								disabled={formData.endpoints.length === 1}
								aria-label="Remove endpoint"
							>
								<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
					<button class="btn btn-sm btn-ghost gap-2" onclick={addEndpoint}>
						<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
						</svg>
						Add Endpoint
					</button>
				</div>
				
				<div class="grid grid-cols-2 gap-4">
					<div class="form-control">
						<label class="label" for="health-path">
							<span class="label-text">Health Path</span>
						</label>
						<input
							id="health-path"
							type="text"
							class="input input-bordered"
							bind:value={formData.health_path}
							placeholder="/health"
						/>
					</div>
					
					<div class="form-control">
						<label class="label" for="service-weight">
							<span class="label-text">Weight</span>
						</label>
						<input
							id="service-weight"
							type="number"
							class="input input-bordered"
							bind:value={formData.weight}
							min="1"
						/>
					</div>
				</div>
				
				<div class="form-control">
					<label class="label" for="service-timeout">
						<span class="label-text">Timeout</span>
					</label>
					<input
						id="service-timeout"
						type="text"
						class="input input-bordered"
						bind:value={formData.timeout}
						placeholder="30s"
					/>
				</div>
				
				<div class="form-control">
					<label class="label cursor-pointer">
						<span class="label-text">Active</span>
						<input
							type="checkbox"
							class="toggle toggle-primary"
							bind:checked={formData.active}
						/>
					</label>
				</div>
			</div>
			
			<div class="modal-action">
				<button class="btn" onclick={closeModal}>Cancel</button>
				<button class="btn btn-primary" onclick={saveService}>Save</button>
			</div>
		</div>
		<form method="dialog" class="modal-backdrop">
			<button onclick={closeModal}>close</button>
		</form>
	</dialog>
{/if}