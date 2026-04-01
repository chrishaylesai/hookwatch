<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { getAuth, loadAuth, setUser } from '$lib/auth.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import type { AuthUser } from '$lib/types';

	const auth = getAuth();

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let submitting = $state(false);

	const redirectTo = $derived.by(() => {
		if (!browser) return '/';
		const url = new URL(window.location.href);
		return url.searchParams.get('redirect') || '/';
	});

	$effect(() => {
		if (auth.loaded && !auth.authEnabled) {
			goto('/');
		}
		if (auth.loaded && auth.isAuthenticated) {
			goto(redirectTo);
		}
	});

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		submitting = true;

		try {
			const response = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: email.trim(), password })
			});

			const payload = await response.json().catch(() => null);

			if (!response.ok) {
				error = payload?.error ?? 'Login failed';
				return;
			}

			setUser(payload as AuthUser);
			await goto(redirectTo);
		} catch {
			error = 'Network error. Please try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>HookWatch | Sign in</title>
</svelte:head>

<div class="mx-auto flex min-h-[calc(100vh-4rem)] max-w-md flex-col items-center justify-center px-4 py-12">
	<div class="mb-8 text-center">
		<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] text-base font-semibold">
			HW
		</div>
		<h1 class="text-3xl font-bold tracking-tight">Sign in</h1>
		<p class="mt-2 text-sm text-[var(--muted-foreground)]">
			Enter your credentials to access HookWatch.
		</p>
	</div>

	<Card class="w-full space-y-5 p-6">
		<form class="space-y-4" onsubmit={handleSubmit}>
			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
					Email
				</span>
				<input
					class="w-full rounded-md border border-[var(--border)] bg-[var(--card)] px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="email"
					autocomplete="email"
					required
					bind:value={email}
				/>
			</label>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
					Password
				</span>
				<input
					class="w-full rounded-md border border-[var(--border)] bg-[var(--card)] px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="password"
					autocomplete="current-password"
					required
					bind:value={password}
				/>
			</label>

			{#if error}
				<div class="rounded-lg border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
					{error}
				</div>
			{/if}

			<Button type="submit" disabled={submitting} class="w-full">
				{submitting ? 'Signing in...' : 'Sign in'}
			</Button>
		</form>

		<p class="text-center text-sm text-[var(--muted-foreground)]">
			Don't have an account?
			<a href="/register" class="font-medium text-[var(--accent-strong)] hover:underline">Register</a>
		</p>
	</Card>
</div>
