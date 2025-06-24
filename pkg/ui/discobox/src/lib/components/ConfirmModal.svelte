<script lang="ts">
	interface Props {
		open: boolean;
		title?: string;
		message: string;
		confirmText?: string;
		cancelText?: string;
		dangerous?: boolean;
		onConfirm: () => void;
		onCancel: () => void;
	}

	let { 
		open = $bindable(false),
		title = 'Confirm Action',
		message,
		confirmText = 'Confirm',
		cancelText = 'Cancel',
		dangerous = false,
		onConfirm,
		onCancel
	}: Props = $props();

	function handleConfirm() {
		onConfirm();
		open = false;
	}

	function handleCancel() {
		onCancel();
		open = false;
	}
</script>

<dialog class="modal" class:modal-open={open}>
	<div class="modal-box">
		<h3 class="font-bold text-lg">{title}</h3>
		<p class="py-4">{message}</p>
		<div class="modal-action">
			<button class="btn btn-ghost" onclick={handleCancel}>
				{cancelText}
			</button>
			<button 
				class="btn"
				class:btn-error={dangerous}
				class:btn-primary={!dangerous}
				onclick={handleConfirm}
			>
				{confirmText}
			</button>
		</div>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button onclick={handleCancel}>close</button>
	</form>
</dialog>