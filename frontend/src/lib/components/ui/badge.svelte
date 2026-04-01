<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	type Tone = 'accent' | 'muted' | 'outline';

	const toneClasses: Record<Tone, string> = {
		accent: 'bg-[var(--accent-soft)] text-[var(--accent-strong)]',
		muted: 'bg-black/6 text-[var(--muted-foreground)]',
		outline: 'border border-[var(--border)] bg-transparent text-[var(--foreground)]'
	};

	let {
		children,
		class: className = '',
		tone = 'accent'
	}: {
		children?: Snippet;
		class?: string;
		tone?: Tone;
	} = $props();
</script>

<span
	class={cn(
		'inline-flex items-center gap-2 rounded-md px-2.5 py-1 text-xs font-semibold uppercase tracking-[0.05em]',
		toneClasses[tone],
		className
	)}
>
	{@render children?.()}
</span>
