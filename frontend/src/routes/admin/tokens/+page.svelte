<script lang="ts">
	import { goto } from '$app/navigation';
	import { getAuth } from '$lib/auth.svelte';
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import Modal from '$lib/components/ui/modal.svelte';
	import type { TokenListResponse, TokenResponse } from '$lib/types';

	type APIError = {
		error?: string;
	};

	const auth = getAuth();
	const perPage = 20;

	let tokens = $state<TokenResponse[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError = $state('');
	let page = $state(1);
	let deleteTarget = $state<TokenResponse | null>(null);
	let deleteError = $state('');
	let deleting = $state(false);

	$effect(() => {
		if (!auth.loaded) {
			return;
		}

		if (!auth.authEnabled) {
			goto('/');
			return;
		}
		if (!auth.isAdmin) {
			goto(auth.isAuthenticated ? '/tokens' : `/login?redirect=${encodeURIComponent('/admin/tokens')}`);
			return;
		}

		void loadTokens(page);
	});

	async function loadTokens(targetPage: number) {
		loading = true;
		loadError = '';

		try {
			const offset = (targetPage - 1) * perPage;
			const response = await fetch(`/api/admin/tokens?limit=${perPage}&offset=${offset}`);
			const payload = (await response.json().catch(() => null)) as TokenListResponse | APIError | null;

			if (!response.ok) {
				loadError = (payload as APIError | null)?.error ?? 'Failed to load tokens';
				tokens = [];
				total = 0;
				return;
			}

			const list = payload as TokenListResponse;
			tokens = list.data ?? [];
			total = list.total ?? 0;
		} catch {
			loadError = 'Network error loading tokens.';
			tokens = [];
			total = 0;
		} finally {
			loading = false;
		}
	}

	function confirmDelete(token: TokenResponse) {
		deleteTarget = token;
		deleteError = '';
	}

	function cancelDelete() {
		if (deleting) return;
		deleteTarget = null;
		deleteError = '';
	}

	async function executeDelete() {
		if (!deleteTarget) return;

		deleting = true;
		deleteError = '';

		try {
			const response = await fetch(`/api/tokens/${deleteTarget.uuid}`, {
				method: 'DELETE'
			});
			const payload = (await response.json().catch(() => null)) as APIError | null;

			if (!response.ok) {
				deleteError = payload?.error ?? 'Failed to delete token';
				return;
			}

			deleteTarget = null;
			if (tokens.length === 1 && page > 1) {
				page -= 1;
				return;
			}
			await loadTokens(page);
		} catch {
			deleteError = 'Network error deleting token.';
		} finally {
			deleting = false;
		}
	}

	function formatTimestamp(value: string) {
		return new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(new Date(value));
	}

	const totalPages = $derived(Math.max(1, Math.ceil(total / perPage)));
</script>

<svelte:head>
	<title>HookWatch | Admin Tokens</title>
</svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8 sm:px-6 lg:px-10">
	<div class="mb-8 flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
		<div>
			<Badge tone="muted">Administration</Badge>
			<h1 class="mt-2 text-3xl font-bold tracking-tight sm:text-4xl">Token management</h1>
			<p class="mt-2 text-sm text-[var(--muted-foreground)]">
				All active tokens across every user account.
			</p>
		</div>
		<div class="flex gap-2">
			<Button href="/admin" variant="ghost" size="sm">Manage users</Button>
			<Button href="/tokens" variant="secondary" size="sm">My tokens</Button>
		</div>
	</div>

	{#if loadError}
		<div class="mb-6 rounded-lg border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
			{loadError}
		</div>
	{/if}

	{#if loading}
		<Card class="px-5 py-8 text-center">
			<p class="text-sm text-[var(--muted-foreground)]">Loading tokens...</p>
		</Card>
	{:else if tokens.length === 0}
		<Card class="px-5 py-8 text-center">
			<p class="text-sm text-[var(--muted-foreground)]">No active tokens found.</p>
		</Card>
	{:else}
		<div class="mb-4 flex items-center justify-between">
			<p class="text-sm text-[var(--muted-foreground)]">
				{total} active {total === 1 ? 'token' : 'tokens'}
			</p>
		</div>

		<div class="space-y-3">
			{#each tokens as token}
				<Card class="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
					<div class="min-w-0 flex-1">
						<div class="flex flex-wrap items-center gap-2">
							<p class="font-mono text-sm font-medium">{token.uuid}</p>
							<Badge tone="accent">admin</Badge>
							<Badge tone="muted">{token.receive_mode} receive</Badge>
							<Badge tone="muted">{token.view_mode} view</Badge>
							{#if token.persistent}
								<Badge tone="accent">persistent</Badge>
							{/if}
						</div>
						<p class="mt-2 text-sm text-[var(--muted-foreground)]">
							Owner: <span class="text-[var(--foreground)]">{token.owner_display ?? 'Anonymous'}</span>
						</p>
						<p class="mt-1 text-sm text-[var(--muted-foreground)]">
							Created {formatTimestamp(token.created_at)}. Updated {formatTimestamp(token.updated_at)}.
						</p>
						<p class="mt-1 text-xs text-[var(--muted-foreground)]">
							{token.persistent ? 'Does not expire.' : `Expires ${formatTimestamp(token.expires_at)}.`}
						</p>
					</div>

					<div class="flex shrink-0 gap-2">
						<Button href={`/tokens/${token.uuid}`} size="sm" variant="secondary">Open</Button>
						<Button type="button" size="sm" variant="ghost" onclick={() => confirmDelete(token)}>
							Delete
						</Button>
					</div>
				</Card>
			{/each}
		</div>

		{#if totalPages > 1}
			<div class="mt-6 flex items-center justify-center gap-4">
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={page <= 1}
					onclick={() => {
						page = Math.max(1, page - 1);
					}}
				>
					Previous
				</Button>
				<span class="text-sm text-[var(--muted-foreground)]">Page {page} of {totalPages}</span>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={page >= totalPages}
					onclick={() => {
						page = Math.min(totalPages, page + 1);
					}}
				>
					Next
				</Button>
			</div>
		{/if}
	{/if}
</div>

<Modal
	open={deleteTarget !== null}
	title="Delete token"
	description="This permanently deletes the webhook token and all captured requests, grants, and actions."
	onclose={cancelDelete}
>
	{#if deleteTarget}
		<div class="space-y-4">
			<p class="text-sm text-[var(--muted-foreground)]">
				Delete <span class="font-mono text-[var(--foreground)]">{deleteTarget.uuid}</span>?
			</p>

			{#if deleteError}
				<div class="rounded-lg border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
					{deleteError}
				</div>
			{/if}

			<div class="flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={cancelDelete} disabled={deleting}>Cancel</Button>
				<Button type="button" onclick={executeDelete} disabled={deleting}>
					{deleting ? 'Deleting...' : 'Delete token'}
				</Button>
			</div>
		</div>
	{/if}
</Modal>
