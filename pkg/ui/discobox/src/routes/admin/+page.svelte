<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { isAuthenticated, isAdmin } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	
	let config = $state<any>(null);
	let loading = $state(true);
	let reloading = $state(false);
	let updating = $state(false);
	
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
					algorithm: config.load_balancing?.algorithm || 'round_robin'
				},
				rateLimit: {
					enabled: config.rate_limit?.enabled || false,
					rps: config.rate_limit?.rps || 1000,
					burst: config.rate_limit?.burst || 2000
				},
				circuitBreaker: {
					enabled: config.circuit_breaker?.enabled || false,
					failureThreshold: config.circuit_breaker?.failure_threshold || 5,
					successThreshold: config.circuit_breaker?.success_threshold || 2,
					timeout: config.circuit_breaker?.timeout || '60s'
				}
			};
		} catch (error) {
			console.error('Failed to load config:', error);
		} finally {
			loading = false;
		}
	}
	
	async function reloadConfig() {
		if (!confirm('This will reload the configuration from disk. Are you sure?')) return;
		
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
			alert(result.message || 'Configuration reloaded successfully');
			await loadConfig();
		} catch (error) {
			console.error('Failed to reload config:', error);
			alert('Failed to reload configuration');
		} finally {
			reloading = false;
		}
	}
	
	async function updateConfig() {
		try {
			updating = true;
			const res = await fetch('/api/v1/admin/config', {
				method: 'PUT',
				headers: {
					'X-API-Key': localStorage.getItem('apiKey') || '',
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(configUpdate)
			});
			
			if (!res.ok) throw new Error('Failed to update config');
			
			const result = await res.json();
			alert(result.message || 'Configuration updated successfully');
			await loadConfig();
		} catch (error) {
			console.error('Failed to update config:', error);
			alert('Failed to update configuration');
		} finally {
			updating = false;
		}
	}
</script>

{#if $isAuthenticated && $isAdmin}
	<Navbar />
	
	<div class="container mx-auto p-4">
		<div class="flex justify-between items-center mb-6">
			<h1 class="text-3xl font-bold">Admin Configuration</h1>
			<button 
				class="btn btn-primary" 
				onclick={reloadConfig}
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
						<h2 class="card-title">Current Configuration</h2>
						<div class="grid gap-4 md:grid-cols-2">
							<div>
								<h3 class="font-semibold mb-2">Server</h3>
								<div class="space-y-1 text-sm">
									<div class="flex justify-between">
										<span class="text-base-content/70">Listen Address</span>
										<span class="font-mono">{config.listen_addr}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-base-content/70">TLS Enabled</span>
										<span>{config.tls?.enabled ? 'Yes' : 'No'}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-base-content/70">HTTP/2 Enabled</span>
										<span>{config.http2?.enabled ? 'Yes' : 'No'}</span>
									</div>
								</div>
							</div>
							
							<div>
								<h3 class="font-semibold mb-2">Features</h3>
								<div class="space-y-1 text-sm">
									<div class="flex justify-between">
										<span class="text-base-content/70">Load Balancing</span>
										<span class="font-mono">{config.load_balancing?.algorithm}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-base-content/70">Sticky Sessions</span>
										<span>{config.load_balancing?.sticky?.enabled ? 'Enabled' : 'Disabled'}</span>
									</div>
									<div class="flex justify-between">
										<span class="text-base-content/70">Rate Limiting</span>
										<span>{config.rate_limit?.enabled ? 'Enabled' : 'Disabled'}</span>
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
						<div class="alert alert-info mb-4">
							<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current shrink-0 w-6 h-6">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
							</svg>
							<span>These settings can be updated without restarting the proxy.</span>
						</div>
						
						<div class="space-y-6">
							<!-- Load Balancing -->
							<div>
								<h3 class="font-semibold mb-2">Load Balancing</h3>
								<div class="form-control">
									<label for="lb-algorithm" class="label">
										<span class="label-text">Algorithm</span>
									</label>
									<select 
										id="lb-algorithm"
										class="select select-bordered"
										bind:value={configUpdate.loadBalancing.algorithm}
									>
										<option value="round_robin">Round Robin</option>
										<option value="weighted">Weighted</option>
										<option value="least_conn">Least Connections</option>
										<option value="ip_hash">IP Hash</option>
									</select>
								</div>
							</div>
							
							<!-- Rate Limiting -->
							<div>
								<h3 class="font-semibold mb-2">Rate Limiting</h3>
								<div class="form-control">
									<label class="label cursor-pointer">
										<span class="label-text">Enable Rate Limiting</span>
										<input 
											type="checkbox" 
											class="toggle toggle-primary"
											bind:checked={configUpdate.rateLimit.enabled}
										/>
									</label>
								</div>
								{#if configUpdate.rateLimit.enabled}
									<div class="grid grid-cols-2 gap-4 mt-2">
										<div class="form-control">
											<label for="rl-rps" class="label">
												<span class="label-text">Requests Per Second</span>
											</label>
											<input 
												id="rl-rps"
												type="number" 
												class="input input-bordered"
												bind:value={configUpdate.rateLimit.rps}
												min="1"
											/>
										</div>
										<div class="form-control">
											<label for="rl-burst" class="label">
												<span class="label-text">Burst Size</span>
											</label>
											<input 
												id="rl-burst"
												type="number" 
												class="input input-bordered"
												bind:value={configUpdate.rateLimit.burst}
												min="1"
											/>
										</div>
									</div>
								{/if}
							</div>
							
							<!-- Circuit Breaker -->
							<div>
								<h3 class="font-semibold mb-2">Circuit Breaker</h3>
								<div class="form-control">
									<label class="label cursor-pointer">
										<span class="label-text">Enable Circuit Breaker</span>
										<input 
											type="checkbox" 
											class="toggle toggle-primary"
											bind:checked={configUpdate.circuitBreaker.enabled}
										/>
									</label>
								</div>
								{#if configUpdate.circuitBreaker.enabled}
									<div class="grid grid-cols-3 gap-4 mt-2">
										<div class="form-control">
											<label for="cb-fail" class="label">
												<span class="label-text">Failure Threshold</span>
											</label>
											<input 
												id="cb-fail"
												type="number" 
												class="input input-bordered"
												bind:value={configUpdate.circuitBreaker.failureThreshold}
												min="1"
											/>
										</div>
										<div class="form-control">
											<label for="cb-success" class="label">
												<span class="label-text">Success Threshold</span>
											</label>
											<input 
												id="cb-success"
												type="number" 
												class="input input-bordered"
												bind:value={configUpdate.circuitBreaker.successThreshold}
												min="1"
											/>
										</div>
										<div class="form-control">
											<label for="cb-timeout" class="label">
												<span class="label-text">Timeout</span>
											</label>
											<input 
												id="cb-timeout"
												type="text" 
												class="input input-bordered"
												bind:value={configUpdate.circuitBreaker.timeout}
												placeholder="60s"
											/>
										</div>
									</div>
								{/if}
							</div>
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