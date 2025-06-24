<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated, isAdmin } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import ConfirmModal from '$lib/components/ConfirmModal.svelte';
	import { toast } from '$lib/stores/toast';
	
	let config = $state<any>(null);
	let loading = $state(true);
	let reloading = $state(false);
	let updating = $state(false);
	let reloadConfirmOpen = $state(false);
	
	let configUpdate = $state({
		loadBalancing: {
			algorithm: ''
		},
		rateLimit: {
			enabled: false,
			rps: 0,
			burst: 0
		},
		circuitBreaker: {
			enabled: false,
			failureThreshold: 0,
			successThreshold: 0,
			timeout: ''
		}
	});
	
	onMount(async () => {
		if (!$isAuthenticated) {
			goto('/login');
			return;
		}
		
		if (!$isAdmin) {
			goto('/');
			return;
		}
		
		await loadConfig();
	});
	
	async function loadConfig() {
		try {
			loading = true;
			const res = await fetch('/api/v1/admin/config', {
				headers: {
					'X-API-Key': localStorage.getItem('apiKey') || ''
				}
			});
			
			if (!res.ok) throw new Error('Failed to load config');
			
			config = await res.json();
			
			// Initialize update form with current values
			configUpdate = {
				loadBalancing: {
					algorithm: config.LoadBalancing?.Algorithm || 'round_robin'
				},
				rateLimit: {
					enabled: config.RateLimit?.Enabled || false,
					rps: config.RateLimit?.RPS || 1000,
					burst: config.RateLimit?.Burst || 2000
				},
				circuitBreaker: {
					enabled: config.CircuitBreaker?.Enabled || false,
					failureThreshold: config.CircuitBreaker?.FailureThreshold || 5,
					successThreshold: config.CircuitBreaker?.SuccessThreshold || 2,
					timeout: config.CircuitBreaker?.Timeout ? `${config.CircuitBreaker.Timeout / 1000000000}s` : '60s'
				}
			};
		} catch (error) {
			console.error('Failed to load config:', error);
		} finally {
			loading = false;
		}
	}
	
	function requestReloadConfig() {
		reloadConfirmOpen = true;
	}
	
	async function reloadConfig() {
		try {
			reloading = true;
			const res = await fetch('/api/v1/admin/reload', {
				method: 'POST',
				headers: {
					'X-API-Key': localStorage.getItem('apiKey') || ''
				}
			});
			
			if (!res.ok) throw new Error('Failed to reload config');
			
			const result = await res.json();
			toast.success(result.message || 'Configuration reloaded successfully');
			await loadConfig();
		} catch (error: any) {
			console.error('Failed to reload config:', error);
			toast.error(error.message || 'Failed to reload configuration');
		} finally {
			reloading = false;
		}
	}
	
	async function updateConfig() {
		try {
			updating = true;
			
			// Transform the config to match the API's expected format
			const configPayload = {
				LoadBalancing: {
					Algorithm: configUpdate.loadBalancing.algorithm
				},
				RateLimit: {
					Enabled: configUpdate.rateLimit.enabled,
					RPS: configUpdate.rateLimit.rps,
					Burst: configUpdate.rateLimit.burst
				},
				CircuitBreaker: {
					Enabled: configUpdate.circuitBreaker.enabled,
					FailureThreshold: configUpdate.circuitBreaker.failureThreshold,
					SuccessThreshold: configUpdate.circuitBreaker.successThreshold,
					Timeout: parseInt(configUpdate.circuitBreaker.timeout.replace(/[^0-9]/g, '')) * 1000000000 // Convert seconds to nanoseconds
				}
			};
			const res = await fetch('/api/v1/admin/config', {
				method: 'PUT',
				headers: {
					'X-API-Key': localStorage.getItem('apiKey') || '',
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(configPayload)
			});
			
			if (!res.ok) {
				const errorText = await res.text();
				throw new Error(errorText || 'Failed to update config');
			}
			
			const result = await res.json();
			toast.success(result.message || 'Configuration updated successfully. You may need to reload from disk to apply changes.');
			await loadConfig();
		} catch (error: any) {
			console.error('Failed to update config:', error);
			toast.error(error.message || 'Failed to update configuration');
		} finally {
			updating = false;
		}
	}
</script>

{#if $isAuthenticated && $isAdmin}
	<Navbar />
	
	<div class="container mx-auto p-4 max-w-7xl">
		<div class="flex justify-between items-center mb-6">
			<h1 class="text-3xl font-bold">Admin Configuration</h1>
			<button 
				class="btn btn-primary" 
				onclick={requestReloadConfig}
				disabled={reloading}
			>
				{#if reloading}
					<span class="loading loading-spinner"></span>
				{/if}
				Reload from Disk
			</button>
		</div>
		
		{#if loading}
			<div class="flex justify-center items-center h-64">
				<span class="loading loading-spinner loading-lg"></span>
			</div>
		{:else if config}
			<div class="grid gap-6">
				<!-- Current Configuration -->
				<div class="card bg-base-200">
					<div class="card-body">
						<h2 class="card-title mb-4">Current Configuration</h2>
						<div class="grid gap-6 lg:grid-cols-2">
							<div>
								<h3 class="font-semibold mb-3 text-sm uppercase text-base-content/70">Server Settings</h3>
								<div class="space-y-3">
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">Listen Address</span>
										<span class="font-mono text-sm font-medium">{config.ListenAddr || ':8080'}</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">TLS Enabled</span>
										<span class="badge badge-sm" class:badge-success={config.TLS?.Enabled} class:badge-ghost={!config.TLS?.Enabled}>
											{config.TLS?.Enabled ? 'Yes' : 'No'}
										</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">HTTP/2 Enabled</span>
										<span class="badge badge-sm" class:badge-success={config.HTTP2?.Enabled} class:badge-ghost={!config.HTTP2?.Enabled}>
											{config.HTTP2?.Enabled ? 'Yes' : 'No'}
										</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">API Address</span>
										<span class="font-mono text-sm font-medium">{config.API?.Addr || ':8081'}</span>
									</div>
								</div>
							</div>
							
							<div>
								<h3 class="font-semibold mb-3 text-sm uppercase text-base-content/70">Features</h3>
								<div class="space-y-3">
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">Load Balancing</span>
										<span class="badge badge-primary">{config.LoadBalancing?.Algorithm || 'round_robin'}</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">Sticky Sessions</span>
										<span class="badge badge-sm" class:badge-success={config.LoadBalancing?.Sticky?.Enabled} class:badge-ghost={!config.LoadBalancing?.Sticky?.Enabled}>
											{config.LoadBalancing?.Sticky?.Enabled ? 'Enabled' : 'Disabled'}
										</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">Rate Limiting</span>
										<span class="badge badge-sm" class:badge-success={config.RateLimit?.Enabled} class:badge-ghost={!config.RateLimit?.Enabled}>
											{config.RateLimit?.Enabled ? `${config.RateLimit?.RPS || 0} req/s` : 'Disabled'}
										</span>
									</div>
									<div class="flex justify-between items-center p-2 rounded hover:bg-base-300 transition-colors">
										<span class="text-sm text-base-content/70">Circuit Breaker</span>
										<span class="badge badge-sm" class:badge-success={config.CircuitBreaker?.Enabled} class:badge-ghost={!config.CircuitBreaker?.Enabled}>
											{config.CircuitBreaker?.Enabled ? 'Enabled' : 'Disabled'}
										</span>
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>
				
				<!-- Runtime Configuration Update -->
				<div class="card bg-base-200">
					<div class="card-body">
						<h2 class="card-title">Runtime Configuration</h2>
						<div class="alert alert-warning mb-4">
							<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current shrink-0 w-6 h-6">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path>
							</svg>
							<div>
								<p class="font-semibold">Runtime changes only!</p>
								<p class="text-sm">These changes update the running configuration but are NOT persisted to disk. To make permanent changes, update the config file and reload.</p>
							</div>
						</div>
						
						<div class="space-y-6">
							<!-- Load Balancing -->
							<fieldset class="fieldset">
								<legend class="fieldset-legend">Load Balancing Algorithm</legend>
								<select class="select" bind:value={configUpdate.loadBalancing.algorithm}>
									<option value="round_robin">Round Robin</option>
									<option value="weighted">Weighted</option>
									<option value="least_conn">Least Connections</option>
									<option value="ip_hash">IP Hash</option>
								</select>
								<p class="label">Select load balancing method</p>
							</fieldset>
							
							<!-- Rate Limiting -->
							<div class="divider"></div>
							<label class="label cursor-pointer">
								<span class="label-text font-semibold">Rate Limiting</span>
								<input type="checkbox" class="checkbox checkbox-primary" bind:checked={configUpdate.rateLimit.enabled} />
							</label>
							{#if configUpdate.rateLimit.enabled}
								<label class="input input-bordered flex items-center gap-2">
									RPS
									<input type="number" class="grow" bind:value={configUpdate.rateLimit.rps} min="1" max="100000" placeholder="1000" />
								</label>
								<label class="input input-bordered flex items-center gap-2">
									Burst
									<input type="number" class="grow" bind:value={configUpdate.rateLimit.burst} min="1" max="200000" placeholder="2000" />
								</label>
							{/if}
							
							<!-- Circuit Breaker -->
							<div class="divider"></div>
							<label class="label cursor-pointer">
								<span class="label-text font-semibold">Circuit Breaker</span>
								<input type="checkbox" class="checkbox checkbox-primary" bind:checked={configUpdate.circuitBreaker.enabled} />
							</label>
							{#if configUpdate.circuitBreaker.enabled}
								<label class="input input-bordered flex items-center gap-2">
									Failures
									<input type="number" class="grow" bind:value={configUpdate.circuitBreaker.failureThreshold} min="1" max="100" placeholder="5" />
								</label>
								<label class="input input-bordered flex items-center gap-2">
									Successes
									<input type="number" class="grow" bind:value={configUpdate.circuitBreaker.successThreshold} min="1" max="50" placeholder="2" />
								</label>
								<label class="input input-bordered flex items-center gap-2">
									Timeout
									<input type="text" class="grow" bind:value={configUpdate.circuitBreaker.timeout} placeholder="60s" />
								</label>
							{/if}
						</div>
						
						<div class="card-actions justify-end mt-6">
							<button 
								class="btn btn-primary"
								onclick={updateConfig}
								disabled={updating}
							>
								{#if updating}
									<span class="loading loading-spinner"></span>
								{/if}
								Apply Changes
							</button>
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
{/if}

<ConfirmModal
	bind:open={reloadConfirmOpen}
	title="Reload Configuration"
	message="This will reload the configuration from disk. Any unsaved runtime changes will be lost. Are you sure?"
	confirmText="Reload"
	cancelText="Cancel"
	onConfirm={reloadConfig}
	onCancel={() => {}}
/>