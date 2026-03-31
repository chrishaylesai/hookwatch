<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	type Variant = 'default' | 'secondary' | 'ghost' | 'outline';
	type Size = 'default' | 'sm' | 'lg';

	const variantClasses: Record<Variant, string> = {
		default:
			'bg-[var(--accent-strong)] text-[var(--accent-foreground)] shadow-[0_18px_40px_rgba(15,37,43,0.18)] hover:-translate-y-0.5 hover:bg-[var(--accent-strong-hover)]',
		secondary:
			'bg-white/88 text-[var(--foreground)] ring-1 ring-black/10 hover:-translate-y-0.5 hover:bg-white',
		ghost:
			'bg-transparent text-[var(--muted-foreground)] hover:bg-white/12 hover:text-[var(--foreground)]',
		outline:
			'border border-white/40 bg-white/8 text-[var(--foreground)] hover:border-white/55 hover:bg-white/14'
	};

	const sizeClasses: Record<Size, string> = {
		default: 'h-11 px-5 text-sm',
		sm: 'h-9 px-4 text-sm',
		lg: 'h-12 px-6 text-base'
	};

	let {
		children,
		class: className = '',
		disabled = false,
		href,
		onclick,
		rel,
		size = 'default',
		target,
		type = 'button',
		variant = 'default'
	}: {
		children?: Snippet;
		class?: string;
		disabled?: boolean;
		href?: string;
		onclick?: (event: MouseEvent) => void;
		rel?: string;
		size?: Size;
		target?: string;
		type?: 'button' | 'submit' | 'reset';
		variant?: Variant;
	} = $props();
</script>

{#if href}
	<a
		class={cn(
			'inline-flex items-center justify-center gap-2 rounded-full font-medium tracking-[0.01em] transition duration-200 ease-out focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--ring)] disabled:pointer-events-none disabled:opacity-55',
			variantClasses[variant],
			sizeClasses[size],
			className
		)}
		href={disabled ? undefined : href}
		{onclick}
		rel={target === '_blank' ? (rel ?? 'noreferrer') : rel}
		target={target}
		aria-disabled={disabled}
	>
		{@render children?.()}
	</a>
{:else}
	<button
		class={cn(
			'inline-flex items-center justify-center gap-2 rounded-full font-medium tracking-[0.01em] transition duration-200 ease-out focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--ring)] disabled:pointer-events-none disabled:opacity-55',
			variantClasses[variant],
			sizeClasses[size],
			className
		)}
		{disabled}
		{onclick}
		{type}
	>
		{@render children?.()}
	</button>
{/if}
