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
				<div class="form-control">
					<label class="label" for="username">
						<span class="label-text">Username</span>
					</label>
					<input
						id="username"
						type="text"
						placeholder="admin"
						class="input input-bordered"
						bind:value={username}
						required
						disabled={loading}
					/>
				</div>
				
				<div class="form-control">
					<label class="label" for="password">
						<span class="label-text">Password</span>
					</label>
					<input
						id="password"
						type="password"
						placeholder="••••••••"
						class="input input-bordered"
						bind:value={password}
						required
						disabled={loading}
					/>
				</div>
				
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