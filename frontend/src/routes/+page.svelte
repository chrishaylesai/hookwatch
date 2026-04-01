<script lang="ts">
	import { browser } from '$app/environment';
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import type { TokenResponse } from '$lib/types';

	type APIError = {
		error?: string;
	};

	const featureCards = [
		{
			title: 'Create endpoints instantly',
			copy: 'Provision a fresh webhook without account setup or page reloads, then start sending traffic immediately.',
			stat: 'One-click token creation'
		},
		{
			title: 'Inspect real traffic',
			copy: 'Headers, query strings, form fields, and bodies are captured by the backend and ready for the next screens.',
			stat: 'Structured request storage'
		},
		{
			title: 'Tune responses later',
			copy: 'Each hook already supports configurable status codes, bodies, timeouts, CORS, and access modes.',
			stat: 'Behavior lives with the token'
		}
	];

	const scaffoldItems = [
		'The home page now calls the real token creation API instead of showing a static mock shell.',
		'Successful creation reveals the full webhook URL, token UUID, expiry, and access defaults.',
		'Copy and open actions make the generated endpoint usable immediately from the landing page.',
		'The page remains the frontend foundation for the next token view and request inspector tasks.'
	];

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

	const createdAtLabel = $derived.by(() =>
		createdToken ? formatTimestamp(createdToken.created_at) : ''
	);

	const expiresAtLabel = $derived.by(() =>
		createdToken ? formatTimestamp(createdToken.expires_at) : ''
	);

	function buildWebhookURL(uuid: string) {
		if (!browser) {
			return `/${uuid}`;
		}

		return new URL(`/${uuid}`, window.location.origin).toString();
	}

	function buildTokenViewURL(uuid: string) {
		return `/tokens/${uuid}`;
	}

	function formatTimestamp(value: string) {
		return new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(new Date(value));
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
	<title>HookWatch | Create a Webhook</title>
</svelte:head>

<div class="relative overflow-hidden">
	<div class="mx-auto flex min-h-screen max-w-7xl flex-col px-4 pb-20 pt-4 sm:px-6 sm:pt-6 lg:px-10">
		<header class="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
			<div class="flex items-center gap-3">
				<div
					class="flex h-11 w-11 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] text-sm font-semibold"
				>
					HW
				</div>
				<div>
					<p class="text-sm font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
						HookWatch
					</p>
					<p class="text-sm text-[var(--muted-foreground)]">Modern webhook workbench</p>
				</div>
			</div>

			<Badge tone="outline" class="self-start sm:self-auto">Token creation live</Badge>
		</header>

		<main class="grid flex-1 gap-10 pt-10 lg:grid-cols-[1.08fr_0.92fr] lg:items-start lg:gap-16">
			<section class="space-y-8">
				<div class="space-y-4">
					<Badge>Home page</Badge>
					<h1 class="max-w-3xl text-4xl font-bold leading-tight tracking-tight text-balance sm:text-5xl lg:text-6xl">
						Create a webhook URL and start sending requests right away.
					</h1>
					<p class="max-w-2xl text-base leading-7 text-[var(--muted-foreground)] sm:text-lg sm:leading-8 lg:text-xl">
						The landing page now provisions a real token through the backend API. Create one,
						copy the generated endpoint, and point Stripe, GitHub, or any local client at it.
					</p>
				</div>

				<div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
					<Button href="#create-webhook" class="w-full sm:w-auto">Create endpoint</Button>
					<Button href="#scaffold" variant="secondary" class="w-full sm:w-auto">What changed</Button>
				</div>

				<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
					{#each featureCards as feature}
						<Card class="space-y-4">
							<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
								{feature.stat}
							</p>
							<div class="space-y-2">
								<h2 class="text-xl font-semibold">{feature.title}</h2>
								<p class="text-sm leading-6 text-[var(--muted-foreground)]">{feature.copy}</p>
							</div>
						</Card>
					{/each}
				</div>
			</section>

			<section id="create-webhook" class="space-y-5">
				<Card class="overflow-hidden p-0">
					<div class="border-b border-[var(--border)] bg-[rgb(14,24,29)] px-5 py-5 text-white sm:px-6">
						<p class="text-xs font-semibold uppercase tracking-[0.05em] text-white/60">
							Create endpoint
						</p>
						<h2 class="mt-3 text-2xl font-semibold">Provision a new webhook token</h2>
						<p class="mt-2 max-w-xl text-sm leading-6 text-white/70">
							New hooks default to public receive and public view mode with a plain text `200`
							response. You can refine that configuration in later screens.
						</p>
					</div>

					<form class="space-y-5 px-5 py-5 sm:px-6 sm:py-6" onsubmit={createWebhook}>
						<div class="grid gap-3 rounded-lg border border-dashed border-[var(--border)] bg-black/[0.02] p-4 sm:grid-cols-2">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
									Receive mode
								</p>
								<p class="mt-2 text-sm font-medium">Public</p>
							</div>
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
									View mode
								</p>
								<p class="mt-2 text-sm font-medium">Public</p>
							</div>
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
									Default response
								</p>
								<p class="mt-2 text-sm font-medium">`200 text/plain`</p>
							</div>
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
									Storage quota
								</p>
								<p class="mt-2 text-sm font-medium">Backend default max requests</p>
							</div>
						</div>

						<div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
							<Button type="submit" disabled={isCreating} class="w-full sm:w-auto">
								{#if isCreating}
									Creating webhook...
								{:else}
									Create new webhook URL
								{/if}
							</Button>
							<p class="text-sm text-[var(--muted-foreground)]">
								The endpoint is available immediately after creation.
							</p>
						</div>

						{#if createError}
							<div
								class="rounded-lg border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800"
								role="alert"
							>
								{createError}
							</div>
						{/if}
					</form>
				</Card>

				<Card class="space-y-5">
					<div class="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
						<div>
							<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
								Generated URL
							</p>
							<h2 class="mt-2 text-2xl font-semibold">Ready-to-use endpoint</h2>
						</div>
						{#if createdToken}
							<Badge>Webhook active</Badge>
						{:else}
							<Badge tone="muted">Waiting for creation</Badge>
						{/if}
					</div>

					{#if createdToken}
						<div class="space-y-4">
							<div class="rounded-lg bg-[rgb(15,25,29)] px-5 py-5 text-white">
								<p class="text-xs font-semibold uppercase tracking-[0.05em] text-white/55">
									Webhook URL
								</p>
								<p class="mt-3 break-all font-mono text-sm leading-7 sm:text-base">{webhookUrl}</p>
								<p class="mt-3 text-sm text-white/60">
									Append any path after the token to organize incoming calls.
								</p>
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
									<p class="text-sm text-[var(--accent-strong)]">Copied to clipboard.</p>
								{:else if copyState === 'error'}
									<p class="text-sm text-amber-700">Clipboard access failed.</p>
								{/if}
							</div>

							<div class="grid gap-3 sm:grid-cols-2">
								<div class="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-4">
									<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
										Token UUID
									</p>
									<p class="mt-2 break-all font-mono text-sm">{createdToken.uuid}</p>
								</div>
								<div class="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-4">
									<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
										Expires
									</p>
									<p class="mt-2 text-sm">{expiresAtLabel}</p>
								</div>
								<div class="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-4">
									<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
										Created
									</p>
									<p class="mt-2 text-sm">{createdAtLabel}</p>
								</div>
								<div class="rounded-lg border border-[var(--border)] bg-[var(--card)] px-4 py-4">
									<p class="text-xs font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
										Default behavior
									</p>
									<p class="mt-2 text-sm">
										{createdToken.default_status} {createdToken.default_content_type}
									</p>
								</div>
							</div>
						</div>
					{:else}
						<div class="rounded-lg border border-dashed border-[var(--border)] bg-black/[0.02] px-5 py-6">
							<p class="text-sm leading-7 text-[var(--muted-foreground)]">
								Create a token to reveal its full webhook URL here. Once generated, you can
								copy it directly into `curl`, test suites, payment provider dashboards, or any
								other webhook sender.
							</p>
						</div>
					{/if}
				</Card>
			</section>
		</main>

		<section
			id="scaffold"
			class="grid gap-5 border-t border-[var(--border)] pt-10 lg:grid-cols-[0.9fr_1.1fr] lg:items-start"
		>
			<div class="space-y-3">
				<Badge tone="muted">Foundation</Badge>
				<h2 class="text-2xl font-bold tracking-tight sm:text-3xl">
					What this home page now handles
				</h2>
				<p class="max-w-xl text-base leading-7 text-[var(--muted-foreground)]">
					The landing page moved from placeholder marketing copy to the first functional user
					flow in the app. The next frontend tasks can build on the created token and route it
					into a dedicated token view.
				</p>
			</div>

			<div class="grid gap-3 sm:grid-cols-2">
				{#each scaffoldItems as item, index}
					<Card class="flex items-start gap-4">
						<div
							class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--accent-soft)] text-sm font-semibold text-[var(--accent-strong)]"
						>
							{index + 1}
						</div>
						<p class="pt-2 text-sm leading-6 text-[var(--foreground)]">{item}</p>
					</Card>
				{/each}
			</div>
		</section>
	</div>
</div>
