<script lang="ts">
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
</script>

<svelte:head>
	<title>HookWatch | Guide</title>
</svelte:head>

<div class="mx-auto max-w-3xl px-4 pb-20 pt-8 sm:px-6 sm:pt-12">
	<div class="space-y-2">
		<h1 class="text-3xl font-bold tracking-tight">How to use HookWatch</h1>
		<p class="text-base text-[var(--muted-foreground)]">
			A quick guide to capturing, inspecting, and configuring webhooks.
		</p>
	</div>

	<div class="mt-10 space-y-8">
		<section class="space-y-3">
			<h2 class="text-xl font-semibold">1. Create a webhook endpoint</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Click the button on the <a href="/" class="underline underline-offset-4 hover:text-[var(--foreground)]">home page</a> to generate a unique webhook URL. No account is required in the default configuration. The endpoint is ready immediately.
			</p>
		</section>

		<section class="space-y-3">
			<h2 class="text-xl font-semibold">2. Send requests to it</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Point any HTTP client at your webhook URL. Use it as a callback URL in Stripe, GitHub, Slack, or any service that sends webhooks. You can also test with cURL:
			</p>
			<Card class="!bg-[rgb(15,25,29)] text-white">
				<pre class="overflow-x-auto font-mono text-sm leading-7 whitespace-pre-wrap">curl -X POST https://your-host/your-token-id \
  -H "Content-Type: application/json" \
  -d '{{"event": "test"}}'</pre>
			</Card>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Any path appended after the token UUID is preserved in the captured request, so you can use paths like <code class="rounded bg-black/5 px-1.5 py-0.5 text-xs">/token-id/webhooks/stripe</code> to organize traffic.
			</p>
		</section>

		<section class="space-y-3">
			<h2 class="text-xl font-semibold">3. Inspect captured traffic</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Open the token view page to see all captured requests in real time via Server-Sent Events. For each request you can inspect:
			</p>
			<ul class="list-inside list-disc space-y-1 text-sm text-[var(--muted-foreground)]">
				<li>HTTP method, URL, and IP address</li>
				<li>Request headers</li>
				<li>Query parameters</li>
				<li>Form data fields</li>
				<li>Raw body with syntax highlighting</li>
				<li>A generated cURL command to replay the request</li>
			</ul>
		</section>

		<section class="space-y-3">
			<h2 class="text-xl font-semibold">4. Configure response behavior</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Each token has configurable defaults for how it responds to incoming requests:
			</p>
			<ul class="list-inside list-disc space-y-1 text-sm text-[var(--muted-foreground)]">
				<li><strong>Status code</strong> &mdash; any valid HTTP status (100&ndash;999)</li>
				<li><strong>Content type</strong> &mdash; the response Content-Type header</li>
				<li><strong>Response body</strong> &mdash; static content returned to the caller</li>
				<li><strong>Timeout</strong> &mdash; delay before responding (0&ndash;10 seconds)</li>
				<li><strong>CORS</strong> &mdash; toggle cross-origin headers on the response</li>
			</ul>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Edit these from the "Edit settings" button on the token view page.
			</p>
		</section>

		<section class="space-y-3">
			<h2 class="text-xl font-semibold">5. Control access</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Hooks support independent access controls for receiving and viewing:
			</p>
			<ul class="list-inside list-disc space-y-1 text-sm text-[var(--muted-foreground)]">
				<li><strong>Receive mode</strong> &mdash; public (anyone can send) or private (requires a secret token via <code class="rounded bg-black/5 px-1.5 py-0.5 text-xs">X-Hook-Secret</code> header, <code class="rounded bg-black/5 px-1.5 py-0.5 text-xs">?secret=</code> query param, or Basic Auth)</li>
				<li><strong>View mode</strong> &mdash; public (anyone can see captured requests) or private (owner and granted users only)</li>
			</ul>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				When authentication is enabled, you can also share hooks with other users by granting them viewer or editor roles.
			</p>
		</section>

		<section class="space-y-3">
			<h2 class="text-xl font-semibold">6. Token expiry</h2>
			<p class="text-sm leading-7 text-[var(--muted-foreground)]">
				Tokens expire automatically after 1 day. The expiry date is shown on the token view page. Expired tokens and their captured requests are cleaned up by the server.
			</p>
		</section>
	</div>

	<div class="mt-12 border-t border-[var(--border)] pt-6">
		<Button href="/" variant="ghost">&larr; Back to home</Button>
	</div>
</div>
