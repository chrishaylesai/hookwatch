<script lang="ts">
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';

	const featureCards = [
		{
			title: 'Capture every request',
			copy: 'Inspect headers, params, and raw bodies without adding temporary logging to your app.',
			stat: 'UUID-backed hook URLs'
		},
		{
			title: 'Respond with intent',
			copy: 'Tune status codes, payloads, timeouts, and CORS from one control surface.',
			stat: 'Per-hook response profiles'
		},
		{
			title: 'Stay live while debugging',
			copy: 'SSE updates keep the request timeline current while the backend persists everything to SQLite.',
			stat: 'Real-time event stream'
		}
	];

	const scaffoldItems = [
		'Tailwind v4 tokens and global theme variables',
		'Shadcn-style UI primitives for buttons, cards, and badges',
		'Static SPA layout configured for Go embed output',
		'Landing page structure ready for token creation and request views'
	];

	const endpointPreview = [
		'POST /api/tokens',
		'GET /api/tokens/{id}',
		'GET /api/tokens/{id}/requests',
		'GET /api/tokens/{id}/events'
	];
</script>

<svelte:head>
	<title>HookWatch | Webhook Workbench</title>
</svelte:head>

<div class="relative overflow-hidden">
	<div
		class="pointer-events-none absolute inset-x-0 top-0 h-[34rem] bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.92),transparent_52%)]"
	></div>

	<div class="mx-auto flex min-h-screen max-w-7xl flex-col px-6 pb-16 pt-6 sm:px-8 lg:px-10">
		<header class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-3">
				<div
					class="flex h-11 w-11 items-center justify-center rounded-full border border-black/10 bg-white/80 text-sm font-semibold shadow-[0_8px_30px_rgba(0,0,0,0.08)]"
				>
					HW
				</div>
				<div>
					<p class="text-sm font-semibold uppercase tracking-[0.28em] text-[var(--muted-foreground)]">
						HookWatch
					</p>
					<p class="text-sm text-[var(--muted-foreground)]">Modern webhook workbench</p>
				</div>
			</div>

			<Badge tone="outline">SvelteKit scaffold</Badge>
		</header>

		<main class="grid flex-1 gap-12 pt-12 lg:grid-cols-[1.2fr_0.8fr] lg:items-center lg:gap-16">
			<section class="space-y-8">
				<div class="space-y-4">
					<Badge>Embedded frontend</Badge>
					<h1 class="max-w-3xl font-[family-name:var(--font-serif)] text-5xl leading-none tracking-[-0.04em] text-balance sm:text-6xl lg:text-7xl">
						Debug webhook traffic in a workspace built for iteration.
					</h1>
					<p class="max-w-2xl text-lg leading-8 text-[var(--muted-foreground)] sm:text-xl">
						This frontend now has a real foundation: Tailwind tokens, reusable UI primitives,
						a production SPA layout, and a landing shell ready for token creation and request
						inspection flows.
					</p>
				</div>

				<div class="flex flex-wrap items-center gap-3">
					<Button href="#scaffold">Review scaffold</Button>
					<Button href="#api-preview" variant="secondary">Inspect API surface</Button>
				</div>

				<div class="grid gap-4 sm:grid-cols-3">
					{#each featureCards as feature}
						<Card class="space-y-4">
							<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
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

			<section class="space-y-5">
				<Card class="overflow-hidden p-0">
					<div class="border-b border-black/8 bg-[rgba(14,24,29,0.9)] px-5 py-4 text-white">
						<p class="text-xs font-semibold uppercase tracking-[0.24em] text-white/60">
							Live preview
						</p>
						<div class="mt-3 space-y-2 font-mono text-sm">
							<p>https://hookwatch.local/3f2504e0-4f89-41d3-9a0c-0305e82c3301</p>
							<p class="text-white/60">listening for incoming requests</p>
						</div>
					</div>
					<div class="grid gap-4 px-5 py-5 sm:grid-cols-2">
						<div class="space-y-2">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								Recent event
							</p>
							<p class="rounded-2xl bg-[var(--accent-soft)] px-4 py-3 font-mono text-sm text-[var(--foreground)]">
								POST /checkout/webhook
							</p>
							<p class="text-sm text-[var(--muted-foreground)]">200 ms • 2.1 KB body • public receive</p>
						</div>
						<div class="space-y-2">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								Response profile
							</p>
							<div class="rounded-2xl border border-dashed border-black/12 px-4 py-3">
								<p class="font-mono text-sm">status=202</p>
								<p class="mt-2 font-mono text-sm">content-type=application/json</p>
							</div>
						</div>
					</div>
				</Card>

				<Card id="api-preview" class="space-y-4">
					<div class="flex items-center justify-between gap-3">
						<div>
							<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
								API preview
							</p>
							<h2 class="mt-2 text-2xl font-semibold">Current backend surface</h2>
						</div>
						<Badge tone="muted">Go + chi</Badge>
					</div>

					<ul class="space-y-2 font-mono text-sm text-[var(--foreground)]">
						{#each endpointPreview as endpoint}
							<li class="rounded-2xl border border-black/8 bg-white/65 px-4 py-3">{endpoint}</li>
						{/each}
					</ul>
				</Card>
			</section>
		</main>

		<section
			id="scaffold"
			class="grid gap-5 border-t border-black/8 pt-10 lg:grid-cols-[0.9fr_1.1fr] lg:items-start"
		>
			<div class="space-y-3">
				<Badge tone="muted">Foundation</Badge>
				<h2 class="font-[family-name:var(--font-serif)] text-4xl tracking-[-0.03em]">
					What this scaffold adds
				</h2>
				<p class="max-w-xl text-base leading-7 text-[var(--muted-foreground)]">
					The next frontend tasks can now build on a coherent visual system instead of the
					default Svelte starter. Token creation, request lists, and detail panels can plug
					into these base primitives directly.
				</p>
			</div>

			<div class="grid gap-3 sm:grid-cols-2">
				{#each scaffoldItems as item, index}
					<Card class="flex items-start gap-4">
						<div
							class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-sm font-semibold text-[var(--accent-strong)]"
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
