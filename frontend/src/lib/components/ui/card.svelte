<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		children,
		class: className = '',
		id
	}: {
		children?: Snippet;
		class?: string;
		id?: string;
	} = $props();
</script>

<section
	{id}
	class={cn(
		'rounded-lg border border-[var(--border)] bg-[var(--card)] p-6',
		className
	)}
>
	{@render children?.()}
</section>
