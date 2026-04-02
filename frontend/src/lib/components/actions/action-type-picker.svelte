<script lang="ts">
	import Modal from '$lib/components/ui/modal.svelte';

	let { onselect, onclose }: { onselect: (type: string) => void; onclose: () => void } = $props();

	const types = [
		{
			type: 'forward',
			label: 'Forward',
			description: 'Send the captured request to another URL',
			color: 'border-sky-200 hover:bg-sky-50'
		},
		{
			type: 'filter',
			label: 'Filter',
			description: 'Continue the pipeline only if a condition is met',
			color: 'border-amber-200 hover:bg-amber-50'
		},
		{
			type: 'delay',
			label: 'Delay',
			description: 'Pause the pipeline for a duration before continuing',
			color: 'border-purple-200 hover:bg-purple-50'
		},
		{
			type: 'transform',
			label: 'Transform',
			description: 'Modify request data for downstream actions',
			color: 'border-emerald-200 hover:bg-emerald-50'
		}
	];
</script>

<Modal onclose={onclose}>
	<div class="w-full max-w-md space-y-5">
		<div>
			<h2 class="text-xl font-semibold">Add action</h2>
			<p class="mt-1 text-sm text-[var(--muted-foreground)]">
				Choose the type of action to add to the pipeline.
			</p>
		</div>

		<div class="grid grid-cols-2 gap-3">
			{#each types as t}
				<button
					type="button"
					onclick={() => onselect(t.type)}
					class="cursor-pointer rounded-lg border bg-[var(--card)] px-4 py-4 text-left transition {t.color}"
				>
					<p class="text-sm font-semibold">{t.label}</p>
					<p class="mt-1 text-xs text-[var(--muted-foreground)]">{t.description}</p>
				</button>
			{/each}
		</div>
	</div>
</Modal>
