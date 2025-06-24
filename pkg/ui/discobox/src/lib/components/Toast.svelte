<script lang="ts">
	import { toast } from '$lib/stores/toast';
	import { fade, fly } from 'svelte/transition';
</script>

<div class="toast toast-end toast-bottom">
	{#each $toast as t (t.id)}
		<div
			class="alert shadow-lg"
			class:alert-success={t.type === 'success'}
			class:alert-error={t.type === 'error'}
			class:alert-warning={t.type === 'warning'}
			class:alert-info={t.type === 'info'}
			transition:fly="{{ y: 100, duration: 300 }}"
		>
			<div>
				{#if t.type === 'success'}
					<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current flex-shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
				{:else if t.type === 'error'}
					<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current flex-shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
					</svg>
				{:else if t.type === 'warning'}
					<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current flex-shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
					</svg>
				{:else}
					<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current flex-shrink-0 w-6 h-6">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
					</svg>
				{/if}
				<span>{t.message}</span>
			</div>
			<div class="flex-none">
				<button class="btn btn-sm btn-ghost" onclick={() => toast.remove(t.id)}>✕</button>
			</div>
		</div>
	{/each}
</div>