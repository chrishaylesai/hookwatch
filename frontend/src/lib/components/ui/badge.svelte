<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	type Tone = 'accent' | 'muted' | 'outline';

	const toneClasses: Record<Tone, string> = {
		accent: 'bg-[var(--accent-soft)] text-[var(--accent-strong)]',
		muted: 'bg-black/6 text-[var(--muted-foreground)]',
		outline: 'border border-white/30 bg-white/8 text-[var(--foreground)]'
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
		'inline-flex items-center gap-2 rounded-full px-3 py-1 text-[0.72rem] font-semibold uppercase tracking-[0.22em]',
		toneClasses[tone],
		className
	)}
>
	{@render children?.()}
</span>
