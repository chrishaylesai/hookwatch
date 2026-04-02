<script lang="ts">
	import { browser } from '$app/environment';
	import Button from '$lib/components/ui/button.svelte';
	import type { TokenResponse } from '$lib/types';

	type APIError = {
		error?: string;
	};

	let createdToken = $state<TokenResponse | null>(null);
	let isCreating = $state(false);
	let createError = $state('');
	let copyState = $state<'idle' | 'done' | 'error'>('idle');

	const webhookUrl = $derived.by(() => {
		if (!createdToken) {
			return '';
		}

		return buildWebhookURL(createdToken.uuid);
	});

	function buildWebhookURL(uuid: string) {
		if (!browser) {
			return `/${uuid}`;
		}

		return new URL(`/${uuid}`, window.location.origin).toString();
	}

	function buildTokenViewURL(uuid: string) {
		return `/tokens/${uuid}`;
	}

	async function createWebhook(event: SubmitEvent) {
		event.preventDefault();

		isCreating = true;
		createError = '';
		copyState = 'idle';

		try {
			const response = await fetch('/api/tokens', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({})
			});

			if (!response.ok) {
				const payload = (await response.json().catch(() => null)) as APIError | null;
				throw new Error(payload?.error ?? 'Failed to create webhook');
			}

			createdToken = (await response.json()) as TokenResponse;
		} catch (error) {
			const message = error instanceof Error ? error.message : 'Failed to create webhook';
			createError = message;
			createdToken = null;
		} finally {
			isCreating = false;
		}
	}

	async function copyWebhookURL() {
		if (!webhookUrl) {
			return;
		}

		try {
			await navigator.clipboard.writeText(webhookUrl);
			copyState = 'done';
		} catch {
			copyState = 'error';
		}
	}
</script>

<svelte:head>
	<title>HookWatch</title>
</svelte:head>

<div class="mx-auto flex min-h-[80vh] max-w-xl flex-col items-center justify-center px-4">
	<div class="flex items-center gap-3">
		<div
			class="flex h-12 w-12 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] text-sm font-semibold"
		>
			HW
		</div>
		<h1 class="text-2xl font-bold tracking-tight">HookWatch</h1>
	</div>

	<p class="mt-4 text-center text-sm text-[var(--muted-foreground)]">
		Create a webhook endpoint and start capturing requests.
	</p>

	{#if createdToken}
		<div class="mt-8 w-full space-y-4">
			<div class="rounded-lg bg-[rgb(15,25,29)] px-5 py-5 text-white">
				<p class="text-xs font-semibold uppercase tracking-[0.05em] text-white/55">
					Your webhook URL
				</p>
				<p class="mt-3 break-all font-mono text-sm leading-7 sm:text-base">{webhookUrl}</p>
			</div>

			<div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
				<Button type="button" onclick={copyWebhookURL} class="w-full sm:w-auto">Copy URL</Button>
				<Button
					href={buildTokenViewURL(createdToken.uuid)}
					variant="secondary"
					class="w-full sm:w-auto"
				>
					Open token view
				</Button>
				{#if copyState === 'done'}
					<p class="text-sm text-[var(--accent-strong)]">Copied.</p>
				{:else if copyState === 'error'}
					<p class="text-sm text-amber-700">Clipboard access failed.</p>
				{/if}
			</div>

			<div class="pt-2">
				<form onsubmit={createWebhook}>
					<Button type="submit" variant="ghost" disabled={isCreating} class="w-full sm:w-auto">
						{isCreating ? 'Creating...' : 'Create another'}
					</Button>
				</form>
			</div>
		</div>
	{:else}
		<form class="mt-8 w-full" onsubmit={createWebhook}>
			<Button type="submit" disabled={isCreating} class="w-full">
				{isCreating ? 'Creating...' : 'Create new webhook URL'}
			</Button>

			{#if createError}
				<div
					class="mt-3 rounded-lg border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800"
					role="alert"
				>
					{createError}
				</div>
			{/if}
		</form>
	{/if}

	<p class="mt-10 text-xs text-[var(--muted-foreground)]">
		<a href="/guide" class="underline decoration-[var(--border)] underline-offset-4 transition hover:text-[var(--foreground)]">
			How it works
		</a>
	</p>
</div>
