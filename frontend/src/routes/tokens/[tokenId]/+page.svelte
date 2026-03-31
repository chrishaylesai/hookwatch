<script lang="ts">
	import { browser } from '$app/environment';
	import { requestToCurl } from '$lib/curl';
	import Badge from '$lib/components/ui/badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import Modal from '$lib/components/ui/modal.svelte';
	import RequestBody from '$lib/components/request-body.svelte';
	import { getAuth } from '$lib/auth.svelte';
	import type {
		HookGrant,
		RequestCreatedEvent,
		RequestListResponse,
		RequestResponse,
		TokenDeletedEvent,
		TokenResponse,
		TokenUpdatedEvent
	} from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TokenSettingsDraft = {
		defaultStatus: string;
		defaultContentType: string;
		defaultContent: string;
		timeout: string;
		cors: boolean;
	};

	type AccessSettingsDraft = {
		receiveMode: 'public' | 'private';
		viewMode: 'public' | 'private';
	};

	let tokenOverride = $state<TokenResponse | null>(null);
	let requestListOverride = $state<RequestListResponse | null>(null);
	let selectedRequestIdOverride = $state<string | null | undefined>(undefined);
	let selectedRequestOverride = $state<RequestResponse | null | undefined>(undefined);
	let webhookCopyState = $state<'idle' | 'done' | 'error'>('idle');
	let curlCopyState = $state<'idle' | 'done' | 'error'>('idle');
	let tokenSettingsOpen = $state(false);
	let tokenSettingsSaving = $state(false);
	let tokenSettingsError = $state('');
	let accessSettingsOpen = $state(false);
	let accessSettingsSaving = $state(false);
	let accessSettingsError = $state('');
	let rotateSecretState = $state<'idle' | 'saving' | 'done' | 'error'>('idle');
	let receiveSecretCopyState = $state<'idle' | 'done' | 'error'>('idle');
	let receiveSecretOverride = $state<string | null | undefined>(undefined);
	let liveUpdatesState = $state<'connecting' | 'live' | 'reconnecting'>('connecting');
	let pendingNewerRequests = $state(0);
	let tokenSettingsDraft = $state<TokenSettingsDraft>({
		defaultStatus: '',
		defaultContentType: '',
		defaultContent: '',
		timeout: '0',
		cors: false
	});
	let accessSettingsDraft = $state<AccessSettingsDraft>({
		receiveMode: 'public',
		viewMode: 'public'
	});

	// Sharing / grant state
	const auth = getAuth();
	let grants = $state<HookGrant[]>([]);
	let grantsLoading = $state(false);
	let grantEmail = $state('');
	let grantRole = $state<'viewer' | 'editor'>('viewer');
	let grantError = $state('');
	let grantAdding = $state(false);

	const currentToken = $derived(tokenOverride ?? data.token);
	const requestList = $derived(requestListOverride ?? data.requestList);
	const selectedRequestId = $derived(
		selectedRequestIdOverride === undefined ? data.selectedRequestId : selectedRequestIdOverride
	);
	const selectedRequest = $derived(
		selectedRequestOverride === undefined ? data.selectedRequest : selectedRequestOverride
	);
	const latestReceiveSecret = $derived(
		receiveSecretOverride === undefined ? currentToken.receive_secret ?? null : receiveSecretOverride
	);
	const webhookUrl = $derived(buildWebhookURL(currentToken.uuid));
	const createdAtLabel = $derived(formatTimestamp(currentToken.created_at));
	const expiresAtLabel = $derived(formatTimestamp(currentToken.expires_at));
	const receiveSecretPrefix = $derived(currentToken.receive_secret_prefix ?? null);
	const curlCommand = $derived.by(() => (selectedRequest ? requestToCurl(selectedRequest) : ''));
	const liveUpdatesLabel = $derived(
		liveUpdatesState === 'live'
			? 'Live'
			: liveUpdatesState === 'reconnecting'
				? 'Reconnecting'
				: 'Connecting'
	);
	const loaderStateKey = $derived.by(() =>
		[
			data.token.uuid,
			data.requestList.page,
			data.requestList.total,
			data.selectedRequestId ?? '',
			data.selectedRequest?.uuid ?? '',
			data.requestList.data.map((request) => request.uuid).join(',')
		].join(':')
	);

	function createTokenSettingsDraft(source: TokenResponse): TokenSettingsDraft {
		return {
			defaultStatus: String(source.default_status),
			defaultContentType: source.default_content_type,
			defaultContent: source.default_content,
			timeout: String(source.timeout),
			cors: source.cors
		};
	}

	function createAccessSettingsDraft(source: TokenResponse): AccessSettingsDraft {
		return {
			receiveMode: source.receive_mode as 'public' | 'private',
			viewMode: source.view_mode as 'public' | 'private'
		};
	}

	function buildWebhookURL(uuid: string) {
		if (!browser) {
			return `/${uuid}`;
		}

		return new URL(`/${uuid}`, window.location.origin).toString();
	}

	function totalPages(total: number, perPage: number) {
		if (total <= 0 || perPage <= 0) {
			return 0;
		}

		return Math.ceil(total / perPage);
	}

	$effect(() => {
		loaderStateKey;
		tokenOverride = null;
		requestListOverride = null;
		selectedRequestIdOverride = undefined;
		selectedRequestOverride = undefined;
		receiveSecretOverride = undefined;
		pendingNewerRequests = 0;
	});

	$effect(() => {
		if (!browser) {
			return;
		}

		const tokenID = data.token.uuid;
		liveUpdatesState = 'connecting';

		const stream = new EventSource(`/api/tokens/${tokenID}/events`);

		stream.onopen = () => {
			liveUpdatesState = 'live';
		};

		stream.onerror = () => {
			liveUpdatesState = 'reconnecting';
		};

		stream.addEventListener('request.created', (event) => {
			const payload = parseEventData<RequestCreatedEvent>(event);
			if (!payload) {
				return;
			}

			liveUpdatesState = 'live';
			applyRequestCreated(payload);
		});

		stream.addEventListener('token.updated', (event) => {
			const payload = parseEventData<TokenUpdatedEvent>(event);
			if (!payload) {
				return;
			}

			liveUpdatesState = 'live';
			tokenOverride = payload.token;
			if (payload.token.receive_mode === 'public') {
				receiveSecretOverride = null;
			}
		});

		stream.addEventListener('token.deleted', (event) => {
			const payload = parseEventData<TokenDeletedEvent>(event);
			if (!payload || payload.token_id !== tokenID) {
				return;
			}

			stream.close();
			window.location.assign('/');
		});

		return () => {
			stream.close();
		};
	});

	function parseEventData<T>(event: Event): T | null {
		if (!(event instanceof MessageEvent) || typeof event.data !== 'string') {
			return null;
		}

		try {
			return JSON.parse(event.data) as T;
		} catch {
			return null;
		}
	}

	function applyRequestCreated(payload: RequestCreatedEvent) {
		const nextTotalPages = totalPages(payload.total, requestList.per_page);

		if (requestList.page === 1) {
			const nextRequests = [
				payload.request,
				...requestList.data.filter((request) => request.uuid !== payload.request.uuid)
			].slice(0, requestList.per_page);

			requestListOverride = {
				...requestList,
				data: nextRequests,
				total: payload.total,
				total_pages: nextTotalPages
			};

			if (!selectedRequestId && nextRequests[0]) {
				selectRequest(nextRequests[0], 'replace');
			}

			return;
		}

		requestListOverride = {
			...requestList,
			total: payload.total,
			total_pages: nextTotalPages
		};
		pendingNewerRequests += 1;
	}

	function updateURL(page: number, requestId: string | null, mode: 'push' | 'replace' = 'push') {
		if (!browser) {
			return;
		}

		const nextURL = new URL(window.location.href);
		nextURL.searchParams.set('page', String(page));

		if (requestId) {
			nextURL.searchParams.set('request', requestId);
		} else {
			nextURL.searchParams.delete('request');
		}

		nextURL.hash = 'requests';

		const method = mode === 'replace' ? 'replaceState' : 'pushState';
		window.history[method](window.history.state, '', `${nextURL.pathname}${nextURL.search}${nextURL.hash}`);
	}

	function isPlainLeftClick(event: MouseEvent) {
		return (
			event.button === 0 &&
			!event.metaKey &&
			!event.ctrlKey &&
			!event.shiftKey &&
			!event.altKey
		);
	}

	function selectRequest(request: RequestResponse, historyMode: 'push' | 'replace' = 'push') {
		selectedRequestIdOverride = request.uuid;
		selectedRequestOverride = request;
		curlCopyState = 'idle';
		updateURL(requestList.page, request.uuid, historyMode);
	}

	function handleRequestClick(event: MouseEvent, request: RequestResponse) {
		if (!isPlainLeftClick(event)) {
			return;
		}

		event.preventDefault();
		selectRequest(request);
	}

	function formatTimestamp(value: string) {
		return new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(new Date(value));
	}

	async function copyWebhookURL() {
		try {
			await navigator.clipboard.writeText(webhookUrl);
			webhookCopyState = 'done';
		} catch {
			webhookCopyState = 'error';
		}
	}

	async function copyCurlCommand() {
		if (!curlCommand) {
			return;
		}

		try {
			await navigator.clipboard.writeText(curlCommand);
			curlCopyState = 'done';
		} catch {
			curlCopyState = 'error';
		}
	}

	async function copyReceiveSecret() {
		if (!latestReceiveSecret) {
			return;
		}

		try {
			await navigator.clipboard.writeText(latestReceiveSecret);
			receiveSecretCopyState = 'done';
		} catch {
			receiveSecretCopyState = 'error';
		}
	}

	function formatListTimestamp(value: string) {
		return new Intl.DateTimeFormat(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		}).format(new Date(value));
	}

	function requestPath(request: RequestResponse) {
		try {
			const parsed = new URL(request.url);
			return `${parsed.pathname}${parsed.search}`;
		} catch {
			return request.url;
		}
	}

	function methodBadgeClass(method: string) {
		switch (method.toUpperCase()) {
			case 'POST':
				return 'bg-emerald-100 text-emerald-800';
			case 'GET':
				return 'bg-sky-100 text-sky-800';
			case 'PUT':
				return 'bg-amber-100 text-amber-800';
			case 'PATCH':
				return 'bg-orange-100 text-orange-800';
			case 'DELETE':
				return 'bg-rose-100 text-rose-800';
			default:
				return 'bg-black/6 text-[var(--muted-foreground)]';
		}
	}

	function requestListHref(page: number, requestId?: string) {
		const params = new URLSearchParams();
		params.set('page', String(page));

		if (requestId) {
			params.set('request', requestId);
		}

		return `?${params.toString()}#requests`;
	}

	function isSelectedRequest(requestId: string) {
		return selectedRequestId === requestId;
	}

	function formatDetailValue(value: unknown): string {
		if (value === null || value === undefined) {
			return '';
		}

		if (Array.isArray(value)) {
			return value.map((item) => formatDetailValue(item)).join(', ');
		}

		if (typeof value === 'object') {
			return JSON.stringify(value, null, 2);
		}

		return String(value);
	}

	function objectEntries(record: Record<string, unknown>): Array<{ key: string; value: string }> {
		return Object.entries(record)
			.map(([key, value]) => ({ key, value: formatDetailValue(value) }))
			.sort((a, b) => a.key.localeCompare(b.key));
	}

	function queryEntries(query: string): Array<{ key: string; value: string }> {
		const params = new URLSearchParams(query);
		const values = new Map<string, string[]>();

		for (const [key, value] of params.entries()) {
			const current = values.get(key) ?? [];
			current.push(value);
			values.set(key, current);
		}

		return Array.from(values.entries())
			.map(([key, value]) => ({ key, value: value.join(', ') }))
			.sort((a, b) => a.key.localeCompare(b.key));
	}

	function openTokenSettingsModal() {
		tokenSettingsDraft = createTokenSettingsDraft(currentToken);
		tokenSettingsError = '';
		tokenSettingsOpen = true;
	}

	function closeTokenSettingsModal() {
		if (tokenSettingsSaving) {
			return;
		}

		tokenSettingsOpen = false;
		tokenSettingsError = '';
	}

	async function loadGrants() {
		if (!auth.authEnabled) return;
		grantsLoading = true;
		try {
			const res = await fetch(`/api/tokens/${currentToken.uuid}/grants`);
			if (res.ok) {
				const payload = await res.json();
				grants = payload.data ?? [];
			}
		} catch {
			// Silently fail — grants are optional
		} finally {
			grantsLoading = false;
		}
	}

	async function addGrant() {
		const trimmedEmail = grantEmail.trim();
		if (!trimmedEmail) {
			grantError = 'Email is required.';
			return;
		}

		grantAdding = true;
		grantError = '';

		try {
			const res = await fetch(`/api/tokens/${currentToken.uuid}/grants`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email: trimmedEmail, role: grantRole })
			});

			if (!res.ok) {
				const payload = await res.json().catch(() => null);
				grantError = payload?.error ?? 'Failed to add grant';
				return;
			}

			grantEmail = '';
			grantRole = 'viewer';
			await loadGrants();
		} catch {
			grantError = 'Network error.';
		} finally {
			grantAdding = false;
		}
	}

	async function removeGrant(userId: string) {
		try {
			await fetch(`/api/tokens/${currentToken.uuid}/grants/${userId}`, {
				method: 'DELETE'
			});
			await loadGrants();
		} catch {
			// Best effort
		}
	}

	function openAccessSettingsModal() {
		accessSettingsDraft = createAccessSettingsDraft(currentToken);
		accessSettingsError = '';
		receiveSecretCopyState = 'idle';
		rotateSecretState = 'idle';
		grantEmail = '';
		grantError = '';
		accessSettingsOpen = true;
		loadGrants();
	}

	function closeAccessSettingsModal() {
		if (accessSettingsSaving || rotateSecretState === 'saving') {
			return;
		}

		accessSettingsOpen = false;
		accessSettingsError = '';
	}

	async function saveTokenSettings(event: SubmitEvent) {
		event.preventDefault();

		const defaultStatus = Number.parseInt(tokenSettingsDraft.defaultStatus, 10);
		if (!Number.isFinite(defaultStatus) || defaultStatus < 100 || defaultStatus > 999) {
			tokenSettingsError = 'Status must be between 100 and 999.';
			return;
		}

		const timeout = Number.parseInt(tokenSettingsDraft.timeout, 10);
		if (!Number.isFinite(timeout) || timeout < 0 || timeout > 10) {
			tokenSettingsError = 'Timeout must be between 0 and 10 seconds.';
			return;
		}

		const contentType = tokenSettingsDraft.defaultContentType.trim();
		if (!contentType) {
			tokenSettingsError = 'Content type must not be empty.';
			return;
		}

		tokenSettingsSaving = true;
		tokenSettingsError = '';

		try {
			const response = await fetch(`/api/tokens/${currentToken.uuid}`, {
				method: 'PUT',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					default_status: defaultStatus,
					default_content_type: contentType,
					default_content: tokenSettingsDraft.defaultContent,
					timeout,
					cors: tokenSettingsDraft.cors
				})
			});

			const payload = (await response.json().catch(() => null)) as
				| (TokenResponse & { error?: undefined })
				| { error?: string }
				| null;

			if (!response.ok) {
				tokenSettingsError = payload && 'error' in payload && payload.error
					? payload.error
					: 'Failed to save token settings.';
				return;
			}

			tokenOverride = payload as TokenResponse;
			tokenSettingsDraft = createTokenSettingsDraft(tokenOverride);
			tokenSettingsOpen = false;
		} catch {
			tokenSettingsError = 'Failed to save token settings.';
		} finally {
			tokenSettingsSaving = false;
		}
	}

	async function saveAccessSettings(event: SubmitEvent) {
		event.preventDefault();

		accessSettingsSaving = true;
		accessSettingsError = '';
		receiveSecretCopyState = 'idle';

		try {
			const response = await fetch(`/api/tokens/${currentToken.uuid}`, {
				method: 'PUT',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					receive_mode: accessSettingsDraft.receiveMode,
					view_mode: accessSettingsDraft.viewMode
				})
			});

			const payload = (await response.json().catch(() => null)) as
				| (TokenResponse & { error?: undefined })
				| { error?: string }
				| null;

			if (!response.ok) {
				accessSettingsError = payload && 'error' in payload && payload.error
					? payload.error
					: 'Failed to save access settings.';
				return;
			}

			tokenOverride = payload as TokenResponse;
			accessSettingsDraft = createAccessSettingsDraft(tokenOverride);
			receiveSecretOverride = tokenOverride.receive_secret ?? receiveSecretOverride;
			if (tokenOverride.receive_mode === 'public') {
				receiveSecretOverride = null;
			}
			accessSettingsOpen = false;
		} catch {
			accessSettingsError = 'Failed to save access settings.';
		} finally {
			accessSettingsSaving = false;
		}
	}

	async function rotateReceiveSecret() {
		if (currentToken.receive_mode !== 'private') {
			return;
		}

		rotateSecretState = 'saving';
		accessSettingsError = '';
		receiveSecretCopyState = 'idle';

		try {
			const response = await fetch(`/api/tokens/${currentToken.uuid}/rotate-secret`, {
				method: 'POST'
			});

			const payload = (await response.json().catch(() => null)) as
				| { receive_secret?: string; receive_secret_prefix?: string; error?: undefined }
				| { error?: string }
				| null;

			if (!response.ok) {
				rotateSecretState = 'error';
				accessSettingsError = payload && 'error' in payload && payload.error
					? payload.error
					: 'Failed to rotate receive secret.';
				return;
			}

			if (payload && 'receive_secret' in payload && payload.receive_secret) {
				receiveSecretOverride = payload.receive_secret;
			}

			if (payload && 'receive_secret_prefix' in payload && payload.receive_secret_prefix) {
				tokenOverride = {
					...currentToken,
					receive_secret_prefix: payload.receive_secret_prefix
				};
			}

			rotateSecretState = 'done';
		} catch {
			rotateSecretState = 'error';
			accessSettingsError = 'Failed to rotate receive secret.';
		}
	}
</script>

<svelte:head>
	<title>HookWatch | Token {currentToken.uuid}</title>
</svelte:head>

<div class="mx-auto flex min-h-screen max-w-7xl flex-col px-4 pb-20 pt-4 sm:px-6 sm:pt-6 lg:px-10">
	<header class="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
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
				<p class="text-sm text-[var(--muted-foreground)]">Token view</p>
			</div>
		</div>

			<Button href="/" variant="secondary" class="w-full sm:w-auto">Create another hook</Button>
		</header>

	<main class="grid flex-1 gap-8 pt-10 lg:grid-cols-[0.84fr_1.16fr]">
		<aside id="requests" class="space-y-5">
			<Card class="space-y-5">
				<div class="flex flex-col items-start justify-between gap-3 sm:flex-row">
					<div>
						<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
							Request list
						</p>
						<h2 class="mt-2 text-2xl font-semibold">Captured traffic</h2>
					</div>
					<div class="flex flex-wrap items-center gap-2 sm:justify-end">
						<Badge tone="muted">{requestList.total} total</Badge>
						<Badge
							tone="muted"
							class={liveUpdatesState === 'live'
								? 'bg-emerald-100 text-emerald-800'
								: 'bg-amber-100 text-amber-800'}
						>
							{liveUpdatesLabel}
						</Badge>
					</div>
				</div>

				{#if pendingNewerRequests > 0}
					<div class="rounded-[24px] border border-amber-200 bg-amber-50 px-4 py-4">
						<p class="text-sm leading-7 text-amber-900">
							{pendingNewerRequests} newer {pendingNewerRequests === 1 ? 'request has' : 'requests have'} arrived on page 1.
							<a
								class="font-semibold underline decoration-amber-400 underline-offset-4"
								href={requestListHref(1)}
							>
								Jump to latest
							</a>
						</p>
					</div>
				{/if}

				{#if requestList.data.length > 0}
					<div class="space-y-3">
						{#each requestList.data as request}
							<a
								href={requestListHref(requestList.page, request.uuid)}
								onclick={(event) => handleRequestClick(event, request)}
								class={`block rounded-[24px] border px-4 py-4 transition duration-200 ease-out ${
									isSelectedRequest(request.uuid)
										? 'border-[var(--accent-strong)] bg-[var(--accent-soft)] shadow-[0_16px_40px_rgba(31,122,106,0.12)]'
										: 'border-black/8 bg-white/68 hover:-translate-y-0.5 hover:bg-white'
								}`}
							>
								<div class="flex items-start justify-between gap-3">
									<div class="min-w-0 flex-1">
										<p class="truncate font-mono text-sm text-[var(--foreground)]">
											{requestPath(request)}
										</p>
										<p class="mt-2 text-xs uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
											{request.hostname}
										</p>
									</div>
									<span
										class={`inline-flex shrink-0 rounded-full px-2.5 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.14em] ${methodBadgeClass(request.method)}`}
									>
										{request.method}
									</span>
								</div>

								<div class="mt-4 flex flex-col items-start gap-1 text-sm text-[var(--muted-foreground)] sm:flex-row sm:items-center sm:justify-between sm:gap-3">
									<span>{formatListTimestamp(request.created_at)}</span>
									<span class="max-w-full truncate font-mono text-xs sm:max-w-[11rem]">
										{request.ip}
									</span>
								</div>
							</a>
						{/each}
					</div>
				{:else}
					<div class="rounded-[28px] border border-dashed border-black/12 bg-black/[0.02] px-5 py-6">
						<p class="text-sm leading-7 text-[var(--muted-foreground)]">
							No requests have been captured for this token yet. Send a request to the webhook
							URL and it will appear here.
						</p>
					</div>
				{/if}

				<div class="flex flex-col gap-3 border-t border-black/8 pt-2 sm:flex-row sm:items-center sm:justify-between">
					<Button
						href={requestListHref(Math.max(1, requestList.page - 1))}
						variant="ghost"
						disabled={requestList.page <= 1}
						class="w-full sm:w-auto"
					>
						Previous
					</Button>
					<p class="text-center text-sm text-[var(--muted-foreground)]">
						Page {requestList.page} of {Math.max(1, requestList.total_pages)}
					</p>
					<Button
						href={requestListHref(
							Math.min(Math.max(1, requestList.total_pages), requestList.page + 1)
						)}
						variant="ghost"
						disabled={requestList.page >= Math.max(1, requestList.total_pages)}
						class="w-full sm:w-auto"
					>
						Next
					</Button>
				</div>
			</Card>
		</aside>

			<section class="space-y-6">
				<div class="space-y-4">
					<Badge>Webhook endpoint</Badge>
					<h1 class="max-w-3xl font-[family-name:var(--font-serif)] text-4xl leading-none tracking-[-0.04em] text-balance sm:text-5xl lg:text-6xl">
						Inspect the selected request without leaving the token page.
					</h1>
					<p class="max-w-2xl text-base leading-7 text-[var(--muted-foreground)] sm:text-lg sm:leading-8">
						The request sidebar now drives a full detail pane. Headers, query params, form data,
						and raw body are all visible next to the webhook URL and token metadata.
					</p>
			</div>

			{#if selectedRequest}
				<Card class="space-y-5">
					<div class="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
						<div>
							<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
								Request detail
							</p>
							<h2 class="mt-2 text-2xl font-semibold">Payload breakdown</h2>
						</div>
						<Badge>{selectedRequest.method}</Badge>
					</div>

					<div class="rounded-[24px] border border-black/8 bg-[var(--accent-soft)] px-4 py-4">
						<div class="flex flex-col items-start gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
									Copy as cURL
								</p>
								<p class="mt-2 text-sm text-[var(--foreground)]">
									Generate a replayable command from the captured method, URL, headers, and
									body.
								</p>
							</div>
							<Button type="button" onclick={copyCurlCommand} size="sm" class="w-full sm:w-auto">Copy cURL</Button>
						</div>

						<div class="mt-4 rounded-[18px] bg-white/72 px-4 py-4">
							<pre class="overflow-x-auto font-mono text-sm leading-6 whitespace-pre-wrap break-words">{curlCommand}</pre>
						</div>

						{#if curlCopyState === 'done'}
							<p class="mt-3 text-sm text-[var(--accent-strong)]">cURL command copied.</p>
						{:else if curlCopyState === 'error'}
							<p class="mt-3 text-sm text-amber-700">Clipboard access failed.</p>
						{/if}
					</div>

					<div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
						<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								Received
							</p>
							<p class="mt-2 text-sm">{formatTimestamp(selectedRequest.created_at)}</p>
						</div>
						<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								IP address
							</p>
							<p class="mt-2 font-mono text-sm">{selectedRequest.ip}</p>
						</div>
						<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								Host
							</p>
							<p class="mt-2 text-sm">{selectedRequest.hostname}</p>
						</div>
						<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
							<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
								User agent
							</p>
							<p class="mt-2 line-clamp-2 text-sm">{selectedRequest.user_agent}</p>
						</div>
					</div>

					<div class="grid gap-5 xl:grid-cols-2">
						<Card class="space-y-4 border-black/6 bg-[rgba(255,255,255,0.55)] p-5 shadow-none">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
									Headers
								</p>
								<h3 class="mt-2 text-lg font-semibold">Request headers</h3>
							</div>

							{#if objectEntries(selectedRequest.headers).length > 0}
								<div class="space-y-3">
									{#each objectEntries(selectedRequest.headers) as entry}
										<div class="rounded-[18px] border border-black/8 bg-white/72 px-4 py-3">
											<p class="font-mono text-xs text-[var(--muted-foreground)]">{entry.key}</p>
											<p class="mt-2 break-words text-sm">{entry.value}</p>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-[var(--muted-foreground)]">No headers captured.</p>
							{/if}
						</Card>

						<Card class="space-y-4 border-black/6 bg-[rgba(255,255,255,0.55)] p-5 shadow-none">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
									Query params
								</p>
								<h3 class="mt-2 text-lg font-semibold">Parsed from request URL</h3>
							</div>

							{#if queryEntries(selectedRequest.query).length > 0}
								<div class="space-y-3">
									{#each queryEntries(selectedRequest.query) as entry}
										<div class="rounded-[18px] border border-black/8 bg-white/72 px-4 py-3">
											<p class="font-mono text-xs text-[var(--muted-foreground)]">{entry.key}</p>
											<p class="mt-2 break-words text-sm">{entry.value}</p>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-[var(--muted-foreground)]">No query parameters.</p>
							{/if}
						</Card>

						<Card class="space-y-4 border-black/6 bg-[rgba(255,255,255,0.55)] p-5 shadow-none">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
									Form data
								</p>
								<h3 class="mt-2 text-lg font-semibold">Parsed key-value fields</h3>
							</div>

							{#if objectEntries(selectedRequest.form_data).length > 0}
								<div class="space-y-3">
									{#each objectEntries(selectedRequest.form_data) as entry}
										<div class="rounded-[18px] border border-black/8 bg-white/72 px-4 py-3">
											<p class="font-mono text-xs text-[var(--muted-foreground)]">{entry.key}</p>
											<p class="mt-2 break-words text-sm">{entry.value}</p>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-sm text-[var(--muted-foreground)]">No form fields captured.</p>
							{/if}
						</Card>

						<Card class="space-y-4 border-black/6 bg-[rgba(15,25,29,0.95)] p-5 text-white shadow-none">
							<div>
								<p class="text-xs font-semibold uppercase tracking-[0.18em] text-white/60">
									Raw body
								</p>
								<h3 class="mt-2 text-lg font-semibold">Highlighted request content</h3>
							</div>

							<RequestBody
								content={selectedRequest.content}
								headers={selectedRequest.headers}
							/>
						</Card>
					</div>
				</Card>
			{:else}
				<Card class="space-y-4">
					<Badge tone="muted">Request detail</Badge>
					<h2 class="text-2xl font-semibold">No request selected</h2>
					<p class="text-sm leading-7 text-[var(--muted-foreground)]">
						Choose a request from the sidebar to inspect its headers, query parameters, form
						data, and raw body here.
					</p>
				</Card>
			{/if}

			<Card class="space-y-5">
				<div class="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-start">
					<div>
						<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
							Token details
						</p>
						<h2 class="mt-2 text-2xl font-semibold">Current configuration</h2>
					</div>
					<div class="flex w-full flex-col items-stretch gap-3 sm:w-auto sm:flex-row sm:flex-wrap sm:items-center sm:justify-end">
						<Badge tone="muted" class="self-start">{currentToken.view_mode} view</Badge>
						<Button type="button" size="sm" variant="secondary" onclick={openTokenSettingsModal} class="w-full sm:w-auto">
							Edit settings
						</Button>
						<Button type="button" size="sm" variant="outline" onclick={openAccessSettingsModal} class="w-full sm:w-auto">
							Access
						</Button>
					</div>
				</div>

				<div class="grid gap-3 sm:grid-cols-2">
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Token UUID
						</p>
						<p class="mt-2 break-all font-mono text-sm">{currentToken.uuid}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Expires
						</p>
						<p class="mt-2 text-sm">{expiresAtLabel}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Created
						</p>
						<p class="mt-2 text-sm">{createdAtLabel}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Default response
						</p>
						<p class="mt-2 text-sm">
							{currentToken.default_status} {currentToken.default_content_type}
						</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Receive mode
						</p>
						<p class="mt-2 text-sm">{currentToken.receive_mode}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							View mode
						</p>
						<p class="mt-2 text-sm">{currentToken.view_mode}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4 sm:col-span-2">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Receive secret
						</p>
						<p class="mt-2 text-sm">
							{#if currentToken.receive_mode === 'private'}
								{#if latestReceiveSecret}
									Available in access settings modal
								{:else if receiveSecretPrefix}
									Stored with prefix `{receiveSecretPrefix}`. Rotate to reveal a new secret.
								{:else}
									Private hook configured
								{/if}
							{:else}
								Not required for public receive mode
							{/if}
						</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Timeout
						</p>
						<p class="mt-2 text-sm">{currentToken.timeout}s</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							CORS
						</p>
						<p class="mt-2 text-sm">{currentToken.cors ? 'Enabled' : 'Disabled'}</p>
					</div>
					<div class="rounded-[24px] border border-black/8 bg-white/70 px-4 py-4 sm:col-span-2">
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Default body
						</p>
						<p class="mt-2 line-clamp-3 whitespace-pre-wrap text-sm">
							{currentToken.default_content || 'Empty response body'}
						</p>
					</div>
				</div>
			</Card>

			<Card class="space-y-5">
				<div class="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
					<div>
						<p class="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted-foreground)]">
							Webhook URL
						</p>
						<h2 class="mt-2 text-2xl font-semibold">Primary endpoint</h2>
					</div>
					<Badge>{currentToken.receive_mode} receive</Badge>
				</div>

				<div class="rounded-[28px] bg-[rgba(15,25,29,0.95)] px-5 py-5 text-white">
					<p class="text-xs font-semibold uppercase tracking-[0.2em] text-white/55">
						Endpoint
					</p>
					<p class="mt-3 break-all font-mono text-sm leading-7 sm:text-base">{webhookUrl}</p>
					<p class="mt-3 text-sm text-white/60">
						Any additional path after the UUID is preserved in captured requests.
					</p>
				</div>

				<div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
					<Button type="button" onclick={copyWebhookURL} class="w-full sm:w-auto">Copy webhook URL</Button>
					<Button href={webhookUrl} target="_blank" variant="outline" class="w-full sm:w-auto">Open endpoint anyway</Button>
					{#if webhookCopyState === 'done'}
						<p class="text-sm text-[var(--accent-strong)]">Copied to clipboard.</p>
					{:else if webhookCopyState === 'error'}
						<p class="text-sm text-amber-700">Clipboard access failed.</p>
					{/if}
				</div>
			</Card>
		</section>
	</main>
</div>

<Modal
	open={tokenSettingsOpen}
	title="Edit response behavior"
	description="Update the token's default status, response body, content type, timeout, and CORS behavior."
	onclose={closeTokenSettingsModal}
>
	<form class="space-y-5" onsubmit={saveTokenSettings}>
		<div class="grid gap-4 sm:grid-cols-2">
			<label class="space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Status code
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="number"
					min="100"
					max="999"
					bind:value={tokenSettingsDraft.defaultStatus}
				/>
			</label>

			<label class="space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Content type
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="text"
					bind:value={tokenSettingsDraft.defaultContentType}
				/>
			</label>

			<label class="space-y-2 sm:col-span-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Default body
				</span>
				<textarea
					class="min-h-36 w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					bind:value={tokenSettingsDraft.defaultContent}
				></textarea>
			</label>

			<label class="space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Timeout
				</span>
				<input
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					type="number"
					min="0"
					max="10"
					bind:value={tokenSettingsDraft.timeout}
				/>
				<p class="text-xs text-[var(--muted-foreground)]">Seconds, from `0` to `10`.</p>
			</label>

			<label class="flex items-center gap-3 self-end rounded-[18px] border border-black/10 bg-white/70 px-4 py-3">
				<input type="checkbox" bind:checked={tokenSettingsDraft.cors} />
				<span class="text-sm">Enable CORS headers on the default response</span>
			</label>
		</div>

		{#if tokenSettingsError}
			<div
				class="rounded-[20px] border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800"
				role="alert"
			>
				{tokenSettingsError}
			</div>
		{/if}

		<div class="flex flex-wrap items-center justify-end gap-3">
			<Button type="button" variant="ghost" onclick={closeTokenSettingsModal} disabled={tokenSettingsSaving}>
				Cancel
			</Button>
			<Button type="submit" disabled={tokenSettingsSaving}>
				{tokenSettingsSaving ? 'Saving...' : 'Save settings'}
			</Button>
		</div>
	</form>
</Modal>

<Modal
	open={accessSettingsOpen}
	title="Manage access behavior"
	description="Control whether receiving the webhook requires a secret, whether the token view is publicly visible, and how the current receive secret is managed."
	onclose={closeAccessSettingsModal}
>
	<form class="space-y-5" onsubmit={saveAccessSettings}>
		<div class="grid gap-4 sm:grid-cols-2">
			<label class="space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					Receive mode
				</span>
				<select
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					bind:value={accessSettingsDraft.receiveMode}
				>
					<option value="public">Public</option>
					<option value="private">Private</option>
				</select>
				<p class="text-xs text-[var(--muted-foreground)]">
					Private mode requires a secret via `X-Hook-Secret`, `?secret=`, or Basic Auth.
				</p>
			</label>

			<label class="space-y-2">
				<span class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
					View mode
				</span>
				<select
					class="w-full rounded-[18px] border border-black/10 bg-white/80 px-4 py-3 text-sm outline-none transition focus:border-[var(--accent-strong)]"
					bind:value={accessSettingsDraft.viewMode}
				>
					<option value="public">Public</option>
					<option value="private">Private</option>
				</select>
				<p class="text-xs text-[var(--muted-foreground)]">
					In no-auth mode the server may force view mode back to `public`.
				</p>
			</label>
		</div>

		<div class="rounded-[24px] border border-black/8 bg-white/72 px-4 py-4">
			<div class="flex flex-wrap items-start justify-between gap-3">
				<div>
					<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
						Receive secret
					</p>
					<p class="mt-2 text-sm leading-7 text-[var(--foreground)]">
						{#if accessSettingsDraft.receiveMode === 'private'}
							Use this secret to deliver requests to a private hook.
						{:else}
							Secrets are disabled while receive mode is public.
						{/if}
					</p>
				</div>

				{#if accessSettingsDraft.receiveMode === 'private'}
					<Button
						type="button"
						size="sm"
						variant="secondary"
						onclick={rotateReceiveSecret}
						disabled={rotateSecretState === 'saving'}
					>
						{rotateSecretState === 'saving' ? 'Rotating...' : 'Rotate secret'}
					</Button>
				{/if}
			</div>

			<div class="mt-4 space-y-3">
				<div class="rounded-[18px] bg-[rgba(15,25,29,0.95)] px-4 py-4 text-white">
					<p class="text-xs font-semibold uppercase tracking-[0.18em] text-white/55">
						Current secret
					</p>
					{#if accessSettingsDraft.receiveMode !== 'private'}
						<p class="mt-2 text-sm text-white/70">Secret not required in public mode.</p>
					{:else if latestReceiveSecret}
						<p class="mt-3 break-all font-mono text-sm leading-7">{latestReceiveSecret}</p>
					{:else if receiveSecretPrefix}
						<p class="mt-3 text-sm text-white/70">
							The full secret is not stored for retrieval. Current prefix: `{receiveSecretPrefix}`.
							Rotate the secret to generate and reveal a new value.
						</p>
					{:else}
						<p class="mt-3 text-sm text-white/70">
							Save private receive mode to generate a new secret.
						</p>
					{/if}
				</div>

				{#if latestReceiveSecret && accessSettingsDraft.receiveMode === 'private'}
					<div class="flex flex-wrap items-center gap-3">
						<Button type="button" size="sm" onclick={copyReceiveSecret}>Copy secret</Button>
						{#if receiveSecretCopyState === 'done'}
							<p class="text-sm text-[var(--accent-strong)]">Secret copied.</p>
						{:else if receiveSecretCopyState === 'error'}
							<p class="text-sm text-amber-700">Clipboard access failed.</p>
						{/if}
						{#if rotateSecretState === 'done'}
							<p class="text-sm text-[var(--accent-strong)]">Secret rotated.</p>
						{:else if rotateSecretState === 'error'}
							<p class="text-sm text-amber-700">Secret rotation failed.</p>
						{/if}
					</div>
				{/if}
			</div>
		</div>

		{#if auth.authEnabled}
			<div class="rounded-[24px] border border-black/8 bg-white/72 px-4 py-4">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div>
						<p class="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
							Sharing
						</p>
						<p class="mt-2 text-sm leading-7 text-[var(--foreground)]">
							Grant other users access to view or edit this hook.
						</p>
					</div>
				</div>

				<div class="mt-4 space-y-3">
					<div class="flex flex-col gap-2 sm:flex-row">
						<input
							class="flex-1 rounded-[18px] border border-black/10 bg-white/80 px-4 py-2.5 text-sm outline-none transition focus:border-[var(--accent-strong)]"
							type="email"
							placeholder="User email"
							bind:value={grantEmail}
						/>
						<select
							class="rounded-[18px] border border-black/10 bg-white/80 px-3 py-2.5 text-sm outline-none"
							bind:value={grantRole}
						>
							<option value="viewer">Viewer</option>
							<option value="editor">Editor</option>
						</select>
						<Button type="button" size="sm" onclick={addGrant} disabled={grantAdding}>
							{grantAdding ? 'Adding...' : 'Add'}
						</Button>
					</div>

					{#if grantError}
						<p class="text-sm text-red-700">{grantError}</p>
					{/if}

					{#if grantsLoading}
						<p class="text-sm text-[var(--muted-foreground)]">Loading grants...</p>
					{:else if grants.length > 0}
						<div class="space-y-2">
							{#each grants as grant}
								<div class="flex items-center justify-between rounded-[18px] border border-black/8 bg-white/60 px-4 py-2.5">
									<div class="min-w-0 flex-1">
										<p class="truncate text-sm">{grant.user_id}</p>
										<p class="text-xs capitalize text-[var(--muted-foreground)]">{grant.role}</p>
									</div>
									<button
										type="button"
										class="ml-3 shrink-0 text-sm text-red-600 hover:text-red-800 hover:underline"
										onclick={() => removeGrant(grant.user_id)}
									>
										Remove
									</button>
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-sm text-[var(--muted-foreground)]">No grants. Only the owner can access this hook.</p>
					{/if}
				</div>
			</div>
		{/if}

		{#if accessSettingsError}
			<div
				class="rounded-[20px] border border-red-300/60 bg-red-50 px-4 py-3 text-sm text-red-800"
				role="alert"
			>
				{accessSettingsError}
			</div>
		{/if}

		<div class="flex flex-wrap items-center justify-end gap-3">
			<Button
				type="button"
				variant="ghost"
				onclick={closeAccessSettingsModal}
				disabled={accessSettingsSaving || rotateSecretState === 'saving'}
			>
				Cancel
			</Button>
			<Button type="submit" disabled={accessSettingsSaving || rotateSecretState === 'saving'}>
				{accessSettingsSaving ? 'Saving...' : 'Save access settings'}
			</Button>
		</div>
	</form>
</Modal>
