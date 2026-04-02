<script lang="ts">
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import ActionCard from './action-card.svelte';
	import ActionEditor from './action-editor.svelte';
	import ActionTypePicker from './action-type-picker.svelte';
	import type { Action, ActionLog } from '$lib/types';

	let { tokenId, actionLogs = [] }: { tokenId: string; actionLogs: ActionLog[] } = $props();

	let actions = $state<Action[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showTypePicker = $state(false);
	let editingAction = $state<Action | null>(null);
	let creatingType = $state<string | null>(null);
	let dragIndex = $state<number | null>(null);

	$effect(() => {
		loadActions();
	});

	async function loadActions() {
		loading = true;
		error = '';
		try {
			const res = await fetch(`/api/tokens/${tokenId}/actions`);
			if (!res.ok) throw new Error('Failed to load actions');
			const data = await res.json();
			actions = data.data ?? [];
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	async function handleDelete(actionId: string) {
		try {
			const res = await fetch(`/api/tokens/${tokenId}/actions/${actionId}`, { method: 'DELETE' });
			if (!res.ok) throw new Error('Failed to delete');
			actions = actions.filter((a) => a.uuid !== actionId);
		} catch {
			error = 'Failed to delete action';
		}
	}

	async function handleToggle(action: Action) {
		try {
			const res = await fetch(`/api/tokens/${tokenId}/actions/${action.uuid}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled: !action.enabled })
			});
			if (!res.ok) throw new Error('Failed to toggle');
			const updated = await res.json();
			actions = actions.map((a) => (a.uuid === updated.uuid ? updated : a));
		} catch {
			error = 'Failed to toggle action';
		}
	}

	function handleEdit(action: Action) {
		editingAction = action;
	}

	function handleTypeSelected(type: string) {
		showTypePicker = false;
		creatingType = type;
	}

	async function handleEditorSave() {
		editingAction = null;
		creatingType = null;
		await loadActions();
	}

	function handleEditorClose() {
		editingAction = null;
		creatingType = null;
	}

	function handleDragStart(index: number) {
		dragIndex = index;
	}

	function handleDragOver(event: DragEvent, index: number) {
		event.preventDefault();
		if (dragIndex === null || dragIndex === index) return;

		const reordered = [...actions];
		const [moved] = reordered.splice(dragIndex, 1);
		reordered.splice(index, 0, moved);
		actions = reordered;
		dragIndex = index;
	}

	async function handleDragEnd() {
		if (dragIndex === null) return;
		dragIndex = null;

		try {
			await fetch(`/api/tokens/${tokenId}/actions/order`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ action_ids: actions.map((a) => a.uuid) })
			});
		} catch {
			error = 'Failed to save order';
			await loadActions();
		}
	}

	function getActionLog(actionId: string): ActionLog | undefined {
		return actionLogs.find((l) => l.action_id === actionId);
	}
</script>

<Card class="space-y-5">
	<div class="flex items-center justify-between">
		<div>
			<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
				Actions
			</p>
			<h2 class="mt-2 text-2xl font-semibold">Pipeline</h2>
		</div>
		<Badge tone="muted">{actions.length} action{actions.length !== 1 ? 's' : ''}</Badge>
	</div>

	{#if loading}
		<p class="text-sm text-[var(--muted-foreground)]">Loading actions...</p>
	{:else if error}
		<div class="rounded-lg border border-rose-200 bg-rose-50 px-4 py-3">
			<p class="text-sm text-rose-800">{error}</p>
		</div>
	{:else if actions.length === 0}
		<div class="rounded-lg border border-dashed border-[var(--border)] bg-black/[0.02] px-5 py-6">
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				No actions configured. Add actions to forward, filter, delay, or transform
				requests after they are captured.
			</p>
		</div>
	{:else}
		<div class="space-y-2">
			{#each actions as action, i}
				<div
					draggable="true"
					ondragstart={() => handleDragStart(i)}
					ondragover={(e) => handleDragOver(e, i)}
					ondragend={handleDragEnd}
					class="transition-opacity {dragIndex === i ? 'opacity-50' : ''}"
				>
					<ActionCard
						{action}
						log={getActionLog(action.uuid)}
						ondelete={() => handleDelete(action.uuid)}
						ontoggle={() => handleToggle(action)}
						onedit={() => handleEdit(action)}
					/>
				</div>
				{#if i < actions.length - 1}
					<div class="flex justify-center py-1">
						<svg class="h-4 w-4 text-[var(--muted-foreground)]" viewBox="0 0 20 20" fill="currentColor">
							<path fill-rule="evenodd" d="M10 3a.75.75 0 0 1 .75.75v10.638l3.96-4.158a.75.75 0 1 1 1.08 1.04l-5.25 5.5a.75.75 0 0 1-1.08 0l-5.25-5.5a.75.75 0 1 1 1.08-1.04l3.96 4.158V3.75A.75.75 0 0 1 10 3Z" clip-rule="evenodd" />
						</svg>
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	<Button type="button" variant="secondary" onclick={() => (showTypePicker = true)} class="w-full">
		Add action
	</Button>
</Card>

{#if showTypePicker}
	<ActionTypePicker
		onselect={handleTypeSelected}
		onclose={() => (showTypePicker = false)}
	/>
{/if}

{#if creatingType}
	<ActionEditor
		{tokenId}
		actionType={creatingType}
		onsave={handleEditorSave}
		onclose={handleEditorClose}
	/>
{/if}

{#if editingAction}
	<ActionEditor
		{tokenId}
		action={editingAction}
		actionType={editingAction.type}
		onsave={handleEditorSave}
		onclose={handleEditorClose}
	/>
{/if}
