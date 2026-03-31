<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		children,
		class: className = '',
		description = '',
		onclose,
		open = false,
		title
	}: {
		children?: Snippet;
		class?: string;
		description?: string;
		onclose?: () => void;
		open?: boolean;
		title: string;
	} = $props();

	function handleBackdropClick() {
		onclose?.();
	}

	function handlePanelClick(event: MouseEvent) {
		event.stopPropagation();
	}

	function handlePanelKeydown(event: KeyboardEvent) {
		event.stopPropagation();
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 flex items-end justify-center bg-[rgba(11,18,22,0.55)] px-3 py-3 backdrop-blur-sm sm:items-center sm:px-4 sm:py-8"
		role="presentation"
		onclick={handleBackdropClick}
	>
		<div
			class={cn(
				'max-h-[calc(100vh-1.5rem)] w-full max-w-2xl overflow-y-auto rounded-[28px] border border-black/10 bg-[rgba(244,239,228,0.96)] p-5 shadow-[0_24px_80px_rgba(0,0,0,0.24)] sm:max-h-[calc(100vh-4rem)] sm:rounded-[32px] sm:p-6',
				className
			)}
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			onclick={handlePanelClick}
			onkeydown={handlePanelKeydown}
		>
			<div class="flex items-start justify-between gap-4">
				<div>
					<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
						Token settings
					</p>
					<h2 class="mt-2 text-xl font-semibold sm:text-2xl">{title}</h2>
					{#if description}
						<p class="mt-3 max-w-xl text-sm leading-7 text-[var(--muted-foreground)]">
							{description}
						</p>
					{/if}
				</div>

				<button
					class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-black/10 bg-white/70 text-lg text-[var(--muted-foreground)] transition hover:bg-white hover:text-[var(--foreground)]"
					type="button"
					aria-label="Close modal"
					onclick={handleBackdropClick}
				>
					×
				</button>
			</div>

			<div class="mt-6">
				{@render children?.()}
			</div>
		</div>
	</div>
{/if}
