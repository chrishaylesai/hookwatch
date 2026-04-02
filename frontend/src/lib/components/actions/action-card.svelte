<script lang="ts">
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import type { Action, ActionLog } from '$lib/types';

	let {
		action,
		log,
		ondelete,
		ontoggle,
		onedit
	}: {
		action: Action;
		log?: ActionLog;
		ondelete: () => void;
		ontoggle: () => void;
		onedit: () => void;
	} = $props();

	const typeColors: Record<string, string> = {
		forward: 'bg-sky-100 text-sky-800',
		filter: 'bg-amber-100 text-amber-800',
		delay: 'bg-purple-100 text-purple-800',
		transform: 'bg-emerald-100 text-emerald-800'
	};

	const statusColors: Record<string, string> = {
		pending: 'bg-black/6 text-[var(--muted-foreground)]',
		running: 'bg-sky-100 text-sky-800',
		success: 'bg-emerald-100 text-emerald-800',
		failed: 'bg-rose-100 text-rose-800',
		skipped: 'bg-black/6 text-[var(--muted-foreground)]'
	};

	function configSummary(action: Action): string {
		const cfg = action.config;
		switch (action.type) {
			case 'forward':
				return (cfg as { url: string }).url;
			case 'filter': {
				const f = cfg as { field: string; operator: string; value?: string; negate?: boolean };
				const neg = f.negate ? 'NOT ' : '';
				return `${neg}${f.field} ${f.operator} ${f.value ?? ''}`.trim();
			}
			case 'delay':
				return `${(cfg as { duration_ms: number }).duration_ms}ms`;
			case 'transform': {
				const t = cfg as { status?: number; content_type?: string; body?: string };
				const parts: string[] = [];
				if (t.status != null) parts.push(`status=${t.status}`);
				if (t.content_type) parts.push(`type=${t.content_type}`);
				if (t.body != null) parts.push('body=...');
				return parts.join(', ') || 'no changes';
			}
			default:
				return '';
		}
	}
</script>

<div
	class="flex items-center gap-3 rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-3 {!action.enabled ? 'opacity-50' : ''}"
>
	<div class="cursor-grab text-[var(--muted-foreground)]" title="Drag to reorder">
		<svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
			<path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4Z" />
		</svg>
	</div>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-2">
			<span
				class="inline-flex rounded-full px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.05em] {typeColors[action.type] ?? ''}"
			>
				{action.type}
			</span>
			{#if log}
				<span
					class="inline-flex rounded-full px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.05em] {statusColors[log.status] ?? ''}"
				>
					{log.status}
				</span>
			{/if}
		</div>
		<p class="mt-1 truncate text-sm text-[var(--muted-foreground)]">{configSummary(action)}</p>
	</div>

	<div class="flex shrink-0 items-center gap-1">
		<button
			type="button"
			onclick={ontoggle}
			class="rounded-md px-2 py-1 text-xs font-medium text-[var(--muted-foreground)] hover:bg-[var(--accent-soft)]"
			title={action.enabled ? 'Disable' : 'Enable'}
		>
			{action.enabled ? 'On' : 'Off'}
		</button>
		<Button type="button" size="sm" variant="ghost" onclick={onedit}>Edit</Button>
		<Button type="button" size="sm" variant="ghost" onclick={ondelete}>Delete</Button>
	</div>
</div>
