<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	
	let services = $state<any[]>([]);
	let loading = $state(true);
	let showModal = $state(false);
	let editingService = $state<any>(null);
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
			services = await api.getServices();
		} catch (error) {
			console.error('Failed to load services:', error);
		} finally {
			loading = false;
		}
	}
	
	function openModal(service?: any) {
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
			alert('Failed to save service');
		}
	}
	
	async function deleteService(id: string) {
		if (!confirm('Are you sure you want to delete this service?')) return;
		
		try {
			await api.deleteService(id);
			await loadServices();
		} catch (error) {
			console.error('Failed to delete service:', error);
			alert('Failed to delete service');
		}
	}
</script>

{#if $isAuthenticated}
	<Navbar />
	
	<div class="container mx-auto p-4">
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
				{#each services as service}
					<div class="card bg-base-200 shadow-xl">
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
							
							<div class="grid gap-2 mt-4">
								<div>
									<span class="font-semibold">Endpoints:</span>
									<ul class="list-disc list-inside ml-4">
										{#each service.endpoints || [] as endpoint}
											<li class="font-mono text-sm">{endpoint}</li>
										{/each}
									</ul>
								</div>
								
								<div class="grid grid-cols-2 md:grid-cols-4 gap-2 text-sm">
									<div>
										<span class="text-base-content/70">Health Path:</span>
										<span class="font-mono ml-2">{service.health_path}</span>
									</div>
									<div>
										<span class="text-base-content/70">Weight:</span>
										<span class="font-mono ml-2">{service.weight}</span>
									</div>
									<div>
										<span class="text-base-content/70">Timeout:</span>
										<span class="font-mono ml-2">{service.timeout}</span>
									</div>
									<div>
										<span class="text-base-content/70">Max Conns:</span>
										<span class="font-mono ml-2">{service.max_conns || 'Unlimited'}</span>
									</div>
								</div>
							</div>
							
							<div class="card-actions justify-end mt-4">
								<button class="btn btn-sm" onclick={() => openModal(service)}>Edit</button>
								<button class="btn btn-sm btn-error" onclick={() => deleteService(service.id)}>Delete</button>
							</div>
						</div>
					</div>
				{/each}
				
				{#if services.length === 0}
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
		<div class="modal-box max-w-2xl">
			<h3 class="font-bold text-lg mb-4">
				{editingService ? 'Edit Service' : 'Add Service'}
			</h3>
			
			<div class="space-y-4">
				<div class="form-control">
					<label class="label">
						<span class="label-text">Service ID</span>
					</label>
					<input
						type="text"
						class="input input-bordered"
						bind:value={formData.id}
						placeholder="my-service"
						disabled={!!editingService}
					/>
				</div>
				
				<div class="form-control">
					<label class="label">
						<span class="label-text">Name</span>
					</label>
					<input
						type="text"
						class="input input-bordered"
						bind:value={formData.name}
						placeholder="My Service"
					/>
				</div>
				
				<div class="form-control">
					<label class="label">
						<span class="label-text">Endpoints</span>
					</label>
					{#each formData.endpoints as endpoint, i}
						<div class="input-group mb-2">
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
							>
								<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
					{/each}
					<button class="btn btn-sm btn-ghost" onclick={addEndpoint}>
						Add Endpoint
					</button>
				</div>
				
				<div class="grid grid-cols-2 gap-4">
					<div class="form-control">
						<label class="label">
							<span class="label-text">Health Path</span>
						</label>
						<input
							type="text"
							class="input input-bordered"
							bind:value={formData.health_path}
							placeholder="/health"
						/>
					</div>
					
					<div class="form-control">
						<label class="label">
							<span class="label-text">Weight</span>
						</label>
						<input
							type="number"
							class="input input-bordered"
							bind:value={formData.weight}
							min="1"
						/>
					</div>
				</div>
				
				<div class="form-control">
					<label class="label">
						<span class="label-text">Timeout</span>
					</label>
					<input
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