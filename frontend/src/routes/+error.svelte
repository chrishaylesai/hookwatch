<script lang="ts">
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { getAuth } from '$lib/auth.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import Badge from '$lib/components/ui/badge.svelte';

	const auth = getAuth();
	const status = $derived($page.status);
	const message = $derived($page.error?.message ?? 'Something went wrong');

	$effect(() => {
		if (!browser) return;

		// Redirect to login for auth errors when auth is enabled
		if (status === 401 && auth.authEnabled && !auth.isAuthenticated) {
			const redirect = encodeURIComponent(window.location.pathname + window.location.search);
			goto(`/login?redirect=${redirect}`);
		}
	});

	const title = $derived.by(() => {
		switch (status) {
			case 401:
				return 'Authentication required';
			case 403:
				return 'Access denied';
			case 404:
				return 'Not found';
			default:
				return 'Error';
		}
	});

	const description = $derived.by(() => {
		switch (status) {
			case 401:
				return 'You need to sign in to access this resource.';
			case 403:
				return 'You do not have permission to access this resource.';
			case 404:
				return 'The page or resource you requested could not be found.';
			default:
				return message;
		}
	});
</script>

<svelte:head>
	<title>HookWatch | {title}</title>
</svelte:head>

<div class="mx-auto flex min-h-[calc(100vh-4rem)] max-w-lg flex-col items-center justify-center px-4 py-12 text-center">
	<Badge tone="muted">{status}</Badge>
	<h1 class="mt-4 font-[family-name:var(--font-serif)] text-3xl tracking-tight sm:text-4xl">
		{title}
	</h1>
	<p class="mt-3 max-w-md text-sm leading-7 text-[var(--muted-foreground)]">
		{description}
	</p>

	<div class="mt-6 flex flex-wrap items-center justify-center gap-3">
		{#if status === 401 && auth.authEnabled}
			<Button href="/login">Sign in</Button>
		{/if}
		<Button href="/" variant="secondary">Go home</Button>
	</div>
</div>
