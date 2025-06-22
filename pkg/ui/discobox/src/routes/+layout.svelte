<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	
	let { children } = $props();
	
	onMount(() => {
		// Set initial theme
		document.documentElement.setAttribute('data-theme', $theme);
		
		// Check auth status
		auth.whoami();
	});
	
	$effect(() => {
		// Update theme when it changes
		document.documentElement.setAttribute('data-theme', $theme);
	});
</script>

<div class="min-h-screen bg-base-100">
	{@render children()}
</div>
