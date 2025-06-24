<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import ConfirmModal from '$lib/components/ConfirmModal.svelte';
	import { toast } from '$lib/stores/toast';
	import type { Service } from '$lib/types';
	
	let services = $state<Service[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingService = $state<Service | null>(null);
	let deleteConfirmOpen = $state(false);
	let serviceToDelete = $state<string | null>(null);
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
	
	function requestDeleteService(id: string) {
		serviceToDelete = id;
		deleteConfirmOpen = true;
	}
	
	async function deleteService() {
		if (!serviceToDelete) return;
		
		try {
			await api.deleteService(serviceToDelete);
			await loadServices();
			toast.success('Service deleted successfully');
		} catch (error: any) {
			console.error('Failed to delete service:', error);
			if (error.message?.includes('referenced by routes')) {
				toast.error('Cannot delete service: It is referenced by one or more routes. Please remove the routes first.');
			} else {
				toast.error('Failed to delete service: ' + (error.message || 'Unknown error'));
			}
		} finally {
			serviceToDelete = null;
			deleteConfirmOpen = false;
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
									<p class="text-sm font-semibold text-base-content/70 mb-2">Endpoints</p>
									<div class="flex flex-wrap gap-2">
										{#each service.endpoints || [] as endpoint}
											<div class="badge badge-outline font-mono text-xs">
												{endpoint}
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
								<button class="btn btn-sm btn-ghost btn-error" onclick={() => requestDeleteService(service.id)}>Delete</button>
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
		<div class="modal-box max-w-3xl">
			<h3 class="text-xl font-bold mb-6">
				{editingService ? 'Edit Service' : 'Add Service'}
			</h3>
			
			<form class="space-y-6">
				<!-- Basic Information -->
				<div class="space-y-4">
					<div class="divider text-sm">Basic Information</div>
					
					<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Service ID</legend>
							<input
								type="text"
								class="input"
								bind:value={formData.id}
								placeholder="my-service"
								disabled={!!editingService}
							/>
							<p class="label">Unique identifier</p>
						</fieldset>
						
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Name</legend>
							<input
								type="text"
								class="input"
								bind:value={formData.name}
								placeholder="My Service"
								required
							/>
							<p class="label">Display name</p>
						</fieldset>
					</div>
				</div>
				
				<!-- Endpoints -->
				<div class="space-y-4">
					<div class="divider text-sm">Endpoints</div>
					
					<fieldset class="fieldset">
						<legend class="fieldset-legend">Backend Endpoints</legend>
						<div class="space-y-2">
							{#each formData.endpoints as endpoint, i}
								<div class="flex gap-2">
									<label class="input flex-1">
										<svg class="h-[1em] opacity-50" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
											<g stroke-linejoin="round" stroke-linecap="round" stroke-width="2.5" fill="none" stroke="currentColor">
												<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path>
												<path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path>
											</g>
										</svg>
										<input
											type="url"
											class="grow"
											bind:value={formData.endpoints[i]}
											placeholder="http://localhost:3000"
											required
										/>
									</label>
									<button
										type="button"
										class="btn btn-square btn-outline btn-error"
										onclick={() => removeEndpoint(i)}
										disabled={formData.endpoints.length === 1}
										aria-label="Remove endpoint"
									>
										<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
										</svg>
									</button>
								</div>
							{/each}
							<button type="button" class="btn btn-sm btn-outline gap-2" onclick={addEndpoint}>
								<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
								</svg>
								Add Endpoint
							</button>
						</div>
						<p class="label">At least one required</p>
					</fieldset>
				</div>
				
				<!-- Configuration -->
				<div class="space-y-4">
					<div class="divider text-sm">Configuration</div>
					
					<div class="grid grid-cols-1 md:grid-cols-2 gap-4">
						<label class="input">
							Health Path
							<input
								type="text"
								class="grow"
								bind:value={formData.health_path}
								placeholder="/health"
							/>
						</label>
						
						<label class="input">
							Timeout
							<input
								type="text"
								class="grow"
								bind:value={formData.timeout}
								placeholder="30s"
								pattern="[0-9]+[smh]"
							/>
						</label>
						
						<fieldset class="fieldset">
							<legend class="fieldset-legend">Weight</legend>
							<input
								type="number"
								class="input"
								bind:value={formData.weight}
								min="1"
								max="100"
								placeholder="1-100"
							/>
							<p class="label">Load balancing weight (1-100)</p>
						</fieldset>
						
						<div class="form-control">
							<label class="label cursor-pointer justify-start gap-3">
								<input
									type="checkbox"
									class="checkbox checkbox-primary"
									bind:checked={formData.active}
								/>
								<span class="label-text">Service Active</span>
							</label>
						</div>
					</div>
				</div>
			</form>
			
			<div class="modal-action">
				<button class="btn btn-ghost" onclick={closeModal}>Cancel</button>
				<button class="btn btn-primary" onclick={saveService}>Save Service</button>
			</div>
		</div>
		<form method="dialog" class="modal-backdrop">
			<button onclick={closeModal}>close</button>
		</form>
	</dialog>
{/if}

<ConfirmModal
	bind:open={deleteConfirmOpen}
	title="Delete Service"
	message="Are you sure you want to delete this service? This action cannot be undone."
	confirmText="Delete"
	cancelText="Cancel"
	dangerous={true}
	onConfirm={deleteService}
	onCancel={() => serviceToDelete = null}
/>