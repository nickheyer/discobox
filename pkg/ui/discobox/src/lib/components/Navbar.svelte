<script lang="ts">
	import { auth, isAdmin } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import { goto } from '$app/navigation';
	
	function logout() {
		auth.logout();
		goto('/login');
	}
</script>

<div class="navbar bg-base-200 shadow-lg sticky top-0 z-50">
	<div class="navbar-start">
		<div class="dropdown lg:hidden">
			<div tabindex="0" role="button" class="btn btn-ghost lg:hidden">
				<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h8m-8 6h16" />
				</svg>
			</div>
			<ul class="menu menu-sm dropdown-content mt-3 z-[1000] p-2 shadow-lg bg-base-100 rounded-box w-52">
				<li><a href="/">Dashboard</a></li>
				<li><a href="/services">Services</a></li>
				<li><a href="/routes">Routes</a></li>
				<li><a href="/metrics">Metrics</a></li>
				{#if $isAdmin}
					<li><a href="/admin">Admin</a></li>
				{/if}
			</ul>
		</div>
		<a href="/" class="btn btn-ghost text-xl font-bold hover:bg-transparent">
			<span class="bg-gradient-to-r from-primary to-secondary bg-clip-text text-transparent">Discobox</span>
		</a>
	</div>
	
	<div class="navbar-center hidden lg:flex">
		<ul class="menu menu-horizontal px-1 gap-1">
			<li><a href="/">Dashboard</a></li>
			<li><a href="/services">Services</a></li>
			<li><a href="/routes">Routes</a></li>
			<li><a href="/metrics">Metrics</a></li>
			{#if $isAdmin}
				<li><a href="/admin">Admin</a></li>
			{/if}
		</ul>
	</div>
	
	<div class="navbar-end">
		<div class="dropdown dropdown-end">
			<div tabindex="0" role="button" class="btn btn-ghost btn-circle">
				<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
				</svg>
			</div>
			<ul class="dropdown-content z-[1000] menu p-2 shadow-lg bg-base-100 rounded-box w-52 max-h-80 overflow-y-auto scrollbar-thin scrollbar-thumb-base-300">
				{#each theme.themes as t}
					<li>
						<button 
							class="justify-start"
							class:active={$theme === t}
							onclick={() => theme.set(t)}
						>
							{t}
						</button>
					</li>
				{/each}
			</ul>
		</div>
		
		<div class="dropdown dropdown-end">
			<div tabindex="0" role="button" class="btn btn-ghost btn-circle avatar hover:bg-base-300 transition-colors">
				<div class="w-10 rounded-full bg-primary text-primary-content grid place-items-center font-semibold">
					{$auth.user?.username.charAt(0).toUpperCase() || 'U'}
				</div>
			</div>
			<ul class="menu menu-sm dropdown-content mt-3 z-[1000] p-2 shadow-lg bg-base-100 rounded-box w-52">
				<li class="menu-title px-4 py-2">
					<span class="text-sm font-semibold">{$auth.user?.username}</span>
				</li>
				<li><button onclick={logout}>Logout</button></li>
			</ul>
		</div>
	</div>
</div>