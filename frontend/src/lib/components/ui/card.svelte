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
		'rounded-[calc(var(--radius)+10px)] border border-black/8 bg-white/72 p-6 shadow-[0_20px_60px_rgba(20,20,20,0.08)] backdrop-blur-xl',
		className
	)}
>
	{@render children?.()}
</section>
