<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { getAuth } from '$lib/auth.svelte';
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import Modal from '$lib/components/ui/modal.svelte';
	import type { AdminUser } from '$lib/types';

	const auth = getAuth();

	let users = $state<AdminUser[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let loadError = $state('');
	let page = $state(1);
	const perPage = 20;

	let editModalOpen = $state(false);
	let editUser = $state<AdminUser | null>(null);
	let editRole = $state('user');
	let editDisplayName = $state('');
	let editSaving = $state(false);
	let editError = $state('');

	let deleteConfirmUser = $state<AdminUser | null>(null);
	let deleteConfirmOpen = $state(false);
	let deleting = $state(false);

	$effect(() => {
		if (auth.loaded && !auth.isAdmin) {
			goto(auth.isAuthenticated ? '/' : `/login?redirect=${encodeURIComponent('/admin')}`);
		}
	});

	$effect(() => {
		if (auth.loaded && auth.isAdmin) {
			loadUsers();
		}
	});

	async function loadUsers() {
		loading = true;
		loadError = '';

		try {
			const offset = (page - 1) * perPage;
			const res = await fetch(`/api/admin/users?limit=${perPage}&offset=${offset}`);

			if (!res.ok) {
				const payload = await res.json().catch(() => null);
				loadError = payload?.error ?? 'Failed to load users';
				return;
			}

			const payload = await res.json();
			users = payload.data ?? [];
			total = payload.total ?? 0;
		} catch {
			loadError = 'Network error loading users.';
		} finally {
			loading = false;
		}
	}

	function openEditModal(user: AdminUser) {
		editUser = user;
		editRole = user.global_role;
		editDisplayName = user.display_name;
		editError = '';
		editModalOpen = true;
	}

	function closeEditModal() {
		if (editSaving) return;
		editModalOpen = false;
		editUser = null;
	}

	async function saveEditUser(event: SubmitEvent) {
		event.preventDefault();
		if (!editUser) return;

		editSaving = true;
		editError = '';

		try {
			const res = await fetch(`/api/admin/users/${editUser.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					display_name: editDisplayName,
					global_role: editRole
				})
			});

			if (!res.ok) {
				const payload = await res.json().catch(() => null);
				editError = payload?.error ?? 'Failed to update user';
				return;
			}

			editModalOpen = false;
			await loadUsers();
		} catch {
			editError = 'Network error.';
		} finally {
			editSaving = false;
		}
	}

	function confirmDelete(user: AdminUser) {
		deleteConfirmUser = user;
		deleteConfirmOpen = true;
	}

	function cancelDelete() {
		if (deleting) return;
		deleteConfirmOpen = false;
		deleteConfirmUser = null;
	}

	async function executeDelete() {
		if (!deleteConfirmUser) return;

		deleting = true;

		try {
			const res = await fetch(`/api/admin/users/${deleteConfirmUser.id}`, {
				method: 'DELETE'
			});

			if (!res.ok) {
				const payload = await res.json().catch(() => null);
				loadError = payload?.error ?? 'Failed to delete user';
			}

			deleteConfirmOpen = false;
			deleteConfirmUser = null;
			await loadUsers();
		} catch {
			loadError = 'Network error deleting user.';
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
	<title>HookWatch | Admin</title>
</svelte:head>

<div class="mx-auto max-w-5xl px-4 py-8 sm:px-6 lg:px-10">
	<div class="mb-8 flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
		<div>
			<Badge tone="muted">Administration</Badge>
			<h1 class="mt-2 font-[family-name:var(--font-serif)] text-3xl tracking-tight sm:text-4xl">
				User management
			</h1>
			<p class="mt-2 text-sm text-[var(--muted-foreground)]">
				{total} registered {total === 1 ? 'user' : 'users'}
			</p>
		</div>
		<Button href="/" variant="secondary" size="sm">Back to home</Button>
	</div>

	{#if loadError}
		<div class="mb-6 rounded-[20px] border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
			{loadError}
		</div>
	{/if}

	{#if loading}
		<Card class="px-5 py-8 text-center">
			<p class="text-sm text-[var(--muted-foreground)]">Loading users...</p>
		</Card>
	{:else if users.length === 0}
		<Card class="px-5 py-8 text-center">
			<p class="text-sm text-[var(--muted-foreground)]">No users found.</p>
		</Card>
	{:else}
		<div class="space-y-3">
			{#each users as user}
				<Card class="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
					<div class="min-w-0 flex-1">
						<div class="flex flex-wrap items-center gap-2">
							<p class="font-medium">{user.display_name || user.email}</p>
							<Badge tone={user.global_role === 'admin' ? 'accent' : 'muted'}>
								{user.global_role}
							</Badge>
							{#if user.oidc_provider}
								<Badge tone="outline">OIDC</Badge>
							{/if}
						</div>
						<p class="mt-1 truncate text-sm text-[var(--muted-foreground)]">{user.email}</p>
						<p class="mt-1 text-xs text-[var(--muted-foreground)]">
							Created {formatTimestamp(user.created_at)}
						</p>
					</div>

					<div class="flex shrink-0 gap-2">
						<Button type="button" size="sm" variant="secondary" onclick={() => openEditModal(user)}>
							Edit
						</Button>
						{#if user.id !== auth.user?.id}
							<Button type="button" size="sm" variant="ghost" onclick={() => confirmDelete(user)}>
								Delete
							</Button>
						{/if}
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
					onclick={() => { page = Math.max(1, page - 1); }}
				>
					Previous
				</Button>
				<span class="text-sm text-[var(--muted-foreground)]">
					Page {page} of {totalPages}
				</span>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={page >= totalPages}
					onclick={() => { page = Math.min(totalPages, page + 1); }}
				>
					Next
				</Button>
			</div>
		{/if}
	{/if}
</div>

<Modal
	open={editModalOpen}
	title="Edit user"
	description="Update the user's display name and role."
	onclose={closeEditModal}
>
	{#if editUser}
		<form class="space-y-4" onsubmit={saveEditUser}>
			<div class="rounded-[18px] border border-black/8 bg-white/60 px-4 py-3">
				<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">Email</p>
				<p class="mt-1 text-sm">{editUser.email}</p>
			</div>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Display name
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="text"
					bind:value={editDisplayName}
				/>
			</label>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Global role
				</span>
				<select
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					bind:value={editRole}
				>
					<option value="user">User</option>
					<option value="admin">Admin</option>
				</select>
			</label>

			{#if editError}
				<div class="rounded-[20px] border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
					{editError}
				</div>
			{/if}

			<div class="flex flex-wrap items-center justify-end gap-3">
				<Button type="button" variant="ghost" onclick={closeEditModal} disabled={editSaving}>Cancel</Button>
				<Button type="submit" disabled={editSaving}>
					{editSaving ? 'Saving...' : 'Save'}
				</Button>
			</div>
		</form>
	{/if}
</Modal>

<Modal
	open={deleteConfirmOpen}
	title="Delete user"
	description="This action cannot be undone. The user's sessions and hook grants will also be removed."
	onclose={cancelDelete}
>
	{#if deleteConfirmUser}
		<div class="space-y-4">
			<div class="rounded-[18px] border border-red-200 bg-red-50 px-4 py-3">
				<p class="text-sm text-red-800">
					Are you sure you want to delete <strong>{deleteConfirmUser.display_name || deleteConfirmUser.email}</strong>?
				</p>
			</div>

			<div class="flex flex-wrap items-center justify-end gap-3">
				<Button type="button" variant="ghost" onclick={cancelDelete} disabled={deleting}>Cancel</Button>
				<Button type="button" onclick={executeDelete} disabled={deleting}
					class="bg-red-600 text-white hover:bg-red-700"
				>
					{deleting ? 'Deleting...' : 'Delete user'}
				</Button>
			</div>
		</div>
	{/if}
</Modal>
