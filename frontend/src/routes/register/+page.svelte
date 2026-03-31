<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { getAuth, setUser } from '$lib/auth.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import type { AuthUser } from '$lib/types';

	const auth = getAuth();

	let email = $state('');
	let displayName = $state('');
	let password = $state('');
	let confirmPassword = $state('');
	let error = $state('');
	let submitting = $state(false);

	$effect(() => {
		if (auth.loaded && !auth.authEnabled) {
			goto('/');
		}
		if (auth.loaded && auth.isAuthenticated) {
			goto('/');
		}
	});

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		error = '';

		if (password !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		submitting = true;

		try {
			const response = await fetch('/api/auth/register', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					email: email.trim(),
					display_name: displayName.trim() || email.trim(),
					password
				})
			});

			const payload = await response.json().catch(() => null);

			if (!response.ok) {
				error = payload?.error ?? 'Registration failed';
				return;
			}

			// Auto-login after registration
			const loginRes = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: email.trim(), password })
			});

			if (loginRes.ok) {
				const user = (await loginRes.json()) as AuthUser;
				setUser(user);
			}

			await goto('/');
		} catch {
			error = 'Network error. Please try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>HookWatch | Register</title>
</svelte:head>

<div class="mx-auto flex min-h-[calc(100vh-4rem)] max-w-md flex-col items-center justify-center px-4 py-12">
	<div class="mb-8 text-center">
		<div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full border border-black/10 bg-white/80 text-base font-semibold shadow-md">
			HW
		</div>
		<h1 class="font-[family-name:var(--font-serif)] text-3xl tracking-tight">Create account</h1>
		<p class="mt-2 text-sm text-[var(--muted-foreground)]">
			Register for a HookWatch account.
		</p>
	</div>

	<Card class="w-full space-y-5 p-6">
		<form class="space-y-4" onsubmit={handleSubmit}>
			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Email
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="email"
					autocomplete="email"
					required
					bind:value={email}
				/>
			</label>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Display name
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="text"
					autocomplete="name"
					placeholder="Optional"
					bind:value={displayName}
				/>
			</label>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Password
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="password"
					autocomplete="new-password"
					required
					minlength="8"
					bind:value={password}
				/>
				<p class="text-xs text-[var(--muted-foreground)]">At least 8 characters.</p>
			</label>

			<label class="block space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Confirm password
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="password"
					autocomplete="new-password"
					required
					bind:value={confirmPassword}
				/>
			</label>

			{#if error}
				<div class="rounded-[20px] border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">
					{error}
				</div>
			{/if}

			<Button type="submit" disabled={submitting} class="w-full">
				{submitting ? 'Creating account...' : 'Create account'}
			</Button>
		</form>

		<p class="text-center text-sm text-[var(--muted-foreground)]">
			Already have an account?
			<a href="/login" class="font-medium text-[var(--accent-strong)] hover:underline">Sign in</a>
		</p>
	</Card>
</div>
