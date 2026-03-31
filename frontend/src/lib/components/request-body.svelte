<script lang="ts">
	import { highlightRequestBody } from '$lib/highlight';

	let {
		content,
		headers
	}: {
		content: string;
		headers: Record<string, unknown>;
	} = $props();

	const highlighted = $derived(highlightRequestBody(content, headers));
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between gap-3">
		<p class="text-xs font-semibold uppercase tracking-[0.18em] text-white/60">
			{highlighted.language === 'plain' ? 'Plain text' : highlighted.language.toUpperCase()}
		</p>
	</div>

	{#if content}
		<pre
			class="hw-code-block overflow-x-auto rounded-[18px] bg-black/20 px-4 py-4 font-mono text-sm leading-6 whitespace-pre-wrap break-words"
		>{@html highlighted.html}</pre>
	{:else}
		<p class="text-sm text-white/70">This request did not include a body.</p>
	{/if}
</div>
