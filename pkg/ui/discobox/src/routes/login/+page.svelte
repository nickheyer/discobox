<script lang="ts">
	import { auth } from '$lib/stores/auth';
	import { goto } from '$app/navigation';
	
	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);
	
	async function handleLogin(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;
		
		try {
			await auth.login(username, password);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<div class="hero min-h-screen bg-base-100">
	<div class="hero-content flex-col">
		<div class="text-center">
			<h1 class="text-5xl font-bold mb-2 bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent">Discobox</h1>
			<p class="text-base-content/70">Production-grade reverse proxy and load balancer</p>
		</div>
		
		<div class="card w-full max-w-sm bg-base-200 shadow-lg mt-8">
			<form class="card-body" onsubmit={handleLogin}>
				<label class="input mb-4">
					<svg class="h-[1em] opacity-50" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
						<g stroke-linejoin="round" stroke-linecap="round" stroke-width="2.5" fill="none" stroke="currentColor">
							<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path>
							<circle cx="12" cy="7" r="4"></circle>
						</g>
					</svg>
					<input
						type="text"
						placeholder="Username"
						class="grow"
						bind:value={username}
						required
						disabled={loading}
					/>
				</label>
				
				<label class="input mb-4">
					<svg class="h-[1em] opacity-50" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
						<g stroke-linejoin="round" stroke-linecap="round" stroke-width="2.5" fill="none" stroke="currentColor">
							<rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
							<path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
						</g>
					</svg>
					<input
						type="password"
						placeholder="Password"
						class="grow"
						bind:value={password}
						required
						disabled={loading}
					/>
				</label>
				
				{#if error}
					<div class="alert alert-error">
						<span>{error}</span>
					</div>
				{/if}
				
				<div class="form-control mt-6">
					<button 
						type="submit" 
						class="btn btn-primary"
						disabled={loading}
					>
						{#if loading}
							<span class="loading loading-spinner"></span>
							Logging in...
						{:else}
							Login
						{/if}
					</button>
				</div>
			</form>
		</div>
	</div>
</div>