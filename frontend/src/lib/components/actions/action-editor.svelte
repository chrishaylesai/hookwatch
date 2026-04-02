<script lang="ts">
	import Button from '$lib/components/ui/button.svelte';
	import Modal from '$lib/components/ui/modal.svelte';
	import type { Action } from '$lib/types';

	let {
		tokenId,
		action = undefined,
		actionType,
		onsave,
		onclose
	}: {
		tokenId: string;
		action?: Action;
		actionType: string;
		onsave: () => void;
		onclose: () => void;
	} = $props();

	const isEditing = !!action;

	// Forward config
	let forwardUrl = $state((action?.config as { url?: string })?.url ?? '');
	let forwardMethod = $state((action?.config as { method?: string })?.method ?? '');
	let forwardTimeout = $state(String((action?.config as { timeout?: number })?.timeout ?? 10));

	// Filter config
	let filterField = $state((action?.config as { field?: string })?.field ?? 'method');
	let filterOperator = $state((action?.config as { operator?: string })?.operator ?? 'equals');
	let filterValue = $state((action?.config as { value?: string })?.value ?? '');
	let filterNegate = $state((action?.config as { negate?: boolean })?.negate ?? false);

	// Delay config
	let delayMs = $state(String((action?.config as { duration_ms?: number })?.duration_ms ?? 1000));

	// Transform config
	let transformStatus = $state(
		(action?.config as { status?: number })?.status != null
			? String((action?.config as { status?: number })?.status)
			: ''
	);
	let transformContentType = $state((action?.config as { content_type?: string })?.content_type ?? '');
	let transformBody = $state((action?.config as { body?: string })?.body ?? '');

	let saving = $state(false);
	let error = $state('');

	function buildConfig(): Record<string, unknown> {
		switch (actionType) {
			case 'forward':
				return {
					url: forwardUrl.trim(),
					method: forwardMethod || undefined,
					timeout: Number.parseInt(forwardTimeout, 10) || 10
				};
			case 'filter':
				return {
					field: filterField,
					operator: filterOperator,
					value: filterValue,
					negate: filterNegate
				};
			case 'delay':
				return { duration_ms: Number.parseInt(delayMs, 10) || 1000 };
			case 'transform': {
				const cfg: Record<string, unknown> = {};
				if (transformStatus.trim()) cfg.status = Number.parseInt(transformStatus, 10);
				if (transformContentType.trim()) cfg.content_type = transformContentType.trim();
				if (transformBody) cfg.body = transformBody;
				return cfg;
			}
			default:
				return {};
		}
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		saving = true;
		error = '';

		const config = buildConfig();
		const url = isEditing
			? `/api/tokens/${tokenId}/actions/${action!.uuid}`
			: `/api/tokens/${tokenId}/actions`;

		try {
			const res = await fetch(url, {
				method: isEditing ? 'PUT' : 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(isEditing ? { config } : { type: actionType, config })
			});

			if (!res.ok) {
				const payload = await res.json().catch(() => null);
				error = payload?.error ?? 'Failed to save action';
				return;
			}

			onsave();
		} catch {
			error = 'Network error';
		} finally {
			saving = false;
		}
	}
</script>

<Modal onclose={onclose}>
	<form onsubmit={handleSubmit} class="w-full max-w-lg space-y-5">
		<div>
			<h2 class="text-xl font-semibold">{isEditing ? 'Edit' : 'Add'} {actionType} action</h2>
		</div>

		{#if actionType === 'forward'}
			<div class="space-y-3">
				<div>
					<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
						URL
					</label>
					<input
						type="url"
						bind:value={forwardUrl}
						required
						placeholder="https://example.com/webhook"
						class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
					/>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Method (optional)
						</label>
						<select
							bind:value={forwardMethod}
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						>
							<option value="">Same as original</option>
							<option value="GET">GET</option>
							<option value="POST">POST</option>
							<option value="PUT">PUT</option>
							<option value="PATCH">PATCH</option>
							<option value="DELETE">DELETE</option>
						</select>
					</div>
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Timeout (seconds)
						</label>
						<input
							type="number"
							bind:value={forwardTimeout}
							min="1"
							max="30"
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						/>
					</div>
				</div>
			</div>
		{:else if actionType === 'filter'}
			<div class="space-y-3">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Field
						</label>
						<select
							bind:value={filterField}
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						>
							<option value="method">Method</option>
							<option value="ip">IP</option>
							<option value="content">Body content</option>
							<option value="header.Content-Type">Header: Content-Type</option>
							<option value="header.User-Agent">Header: User-Agent</option>
							<option value="header.Authorization">Header: Authorization</option>
						</select>
					</div>
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Operator
						</label>
						<select
							bind:value={filterOperator}
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						>
							<option value="equals">Equals</option>
							<option value="contains">Contains</option>
							<option value="matches">Matches (regex)</option>
							<option value="exists">Exists</option>
						</select>
					</div>
				</div>
				{#if filterOperator !== 'exists'}
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Value
						</label>
						<input
							type="text"
							bind:value={filterValue}
							placeholder="Match value..."
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						/>
					</div>
				{/if}
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={filterNegate} class="rounded" />
					Negate (stop if condition matches)
				</label>
			</div>
		{:else if actionType === 'delay'}
			<div>
				<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
					Duration (milliseconds)
				</label>
				<input
					type="number"
					bind:value={delayMs}
					min="100"
					max="30000"
					step="100"
					class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
				/>
				<p class="mt-1 text-xs text-[var(--muted-foreground)]">
					{Number.parseInt(delayMs, 10) >= 1000
						? `${(Number.parseInt(delayMs, 10) / 1000).toFixed(1)}s`
						: `${delayMs}ms`}
				</p>
			</div>
		{:else if actionType === 'transform'}
			<div class="space-y-3">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Status code (optional)
						</label>
						<input
							type="number"
							bind:value={transformStatus}
							min="100"
							max="999"
							placeholder="e.g. 200"
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						/>
					</div>
					<div>
						<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
							Content type (optional)
						</label>
						<input
							type="text"
							bind:value={transformContentType}
							placeholder="application/json"
							class="h-9 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
						/>
					</div>
				</div>
				<div>
					<label class="mb-1 block text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
						Body (optional)
					</label>
					<textarea
						bind:value={transformBody}
						rows={4}
						placeholder="Transformed body content..."
						class="w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 py-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
					></textarea>
				</div>
			</div>
		{/if}

		{#if error}
			<div class="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3">
				<p class="text-sm text-rose-800">{error}</p>
			</div>
		{/if}

		<div class="flex justify-end gap-3">
			<Button type="button" variant="ghost" onclick={onclose}>Cancel</Button>
			<Button type="submit" disabled={saving}>
				{saving ? 'Saving...' : isEditing ? 'Update' : 'Create'}
			</Button>
		</div>
	</form>
</Modal>
