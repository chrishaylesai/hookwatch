<script lang="ts">
	import { browser } from '$app/environment';
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { getAuth, loadAuth, logout } from '$lib/auth.svelte';
	import Button from '$lib/components/ui/button.svelte';

	let { children } = $props();

	type Theme = 'light' | 'dark';

	const themeStorageKey = 'hookwatch-theme';
	const initialTheme: Theme =
		browser && document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';

	let theme = $state<Theme>(initialTheme);
	let userMenuOpen = $state(false);

	const auth = getAuth();

	$effect(() => {
		if (browser) {
			loadAuth();
		}
	});

	function applyTheme(nextTheme: Theme) {
		theme = nextTheme;

		if (!browser) {
			return;
		}

		document.documentElement.dataset.theme = nextTheme;
		document.documentElement.style.colorScheme = nextTheme;
		localStorage.setItem(themeStorageKey, nextTheme);
	}

	function toggleTheme() {
		applyTheme(theme === 'dark' ? 'light' : 'dark');
	}

	function toggleUserMenu() {
		userMenuOpen = !userMenuOpen;
	}

	function closeUserMenu() {
		userMenuOpen = false;
	}

	async function handleLogout() {
		closeUserMenu();
		await logout();
		window.location.assign('/');
	}

	const themeLabel = $derived(theme === 'dark' ? 'Light mode' : 'Dark mode');
	const themeMetaColor = $derived(theme === 'dark' ? '#091217' : '#f4efe4');
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>HookWatch</title>
	<meta
		name="description"
		content="A modern webhook inspector for capturing, replaying, and debugging HTTP traffic."
	/>
	<meta name="theme-color" content={themeMetaColor} />
</svelte:head>

<div class="min-h-screen">
	{#if auth.authEnabled && auth.loaded}
		<nav class="sticky top-0 z-40 border-b border-[var(--border)] bg-[var(--background)]">
			<div class="mx-auto flex max-w-7xl items-center justify-between px-4 py-3 sm:px-6 lg:px-10">
				<a href="/" class="flex items-center gap-2.5">
					<div
						class="flex h-9 w-9 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--card)] text-xs font-semibold"
					>
						HW
					</div>
					<span class="text-sm font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
						HookWatch
					</span>
				</a>

				<div class="flex items-center gap-3">
					{#if auth.isAuthenticated}
						{#if auth.isAdmin}
							<Button href="/admin" variant="ghost" size="sm">Admin</Button>
						{/if}

						<div class="relative">
							<button
								type="button"
								class="flex h-9 items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--card)] px-3 text-sm transition hover:bg-[var(--accent-soft)]"
								onclick={toggleUserMenu}
							>
								<span class="inline-flex h-6 w-6 items-center justify-center rounded-md bg-[var(--accent-soft)] text-xs font-semibold text-[var(--accent-strong)]">
									{auth.user?.display_name?.[0]?.toUpperCase() ?? '?'}
								</span>
								<span class="hidden max-w-[10rem] truncate sm:inline">{auth.user?.display_name ?? auth.user?.email}</span>
								<svg class="h-4 w-4 text-[var(--muted-foreground)]" viewBox="0 0 20 20" fill="currentColor">
									<path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
								</svg>
							</button>

							{#if userMenuOpen}
								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div class="fixed inset-0 z-40" onclick={closeUserMenu} onkeydown={closeUserMenu}></div>
								<div class="absolute right-0 z-50 mt-2 w-56 rounded-lg border border-[var(--border)] bg-[var(--background)] p-1.5 shadow-[0_4px_16px_rgba(0,0,0,0.12)]">
									<div class="px-3 py-2.5">
										<p class="text-sm font-medium">{auth.user?.display_name}</p>
										<p class="mt-0.5 truncate text-xs text-[var(--muted-foreground)]">{auth.user?.email}</p>
										<p class="mt-1 text-xs text-[var(--muted-foreground)]">
											Role: <span class="font-medium capitalize">{auth.user?.global_role}</span>
										</p>
									</div>
									<hr class="my-1 border-[var(--border)]" />
									<button
										type="button"
										class="w-full rounded-md px-3 py-2 text-left text-sm text-[var(--foreground)] transition hover:bg-[var(--accent-soft)]"
										onclick={handleLogout}
									>
										Sign out
									</button>
								</div>
							{/if}
						</div>
					{:else}
						<Button href="/login" variant="ghost" size="sm">Sign in</Button>
						<Button href="/register" size="sm">Register</Button>
					{/if}
				</div>
			</div>
		</nav>
	{/if}

	<button
		type="button"
		class="hw-theme-toggle fixed right-4 bottom-4 z-50 inline-flex items-center gap-2 rounded-full px-3 py-3 text-sm font-medium tracking-[0.01em] transition duration-200 ease-out focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--ring)] sm:right-5 sm:bottom-5 sm:gap-3 sm:px-4"
		aria-label={`Switch to ${themeLabel.toLowerCase()}`}
		title={`Switch to ${themeLabel.toLowerCase()}`}
		onclick={toggleTheme}
	>
		<span
			class="inline-flex h-8 w-8 items-center justify-center rounded-md bg-[var(--accent-soft)] text-[var(--accent-strong)]"
			aria-hidden="true"
		>
			{#if theme === 'dark'}
				<svg viewBox="0 0 24 24" class="h-4 w-4 fill-none stroke-current" stroke-width="1.8">
					<circle cx="12" cy="12" r="4.25"></circle>
					<path d="M12 2.75v2.5M12 18.75v2.5M21.25 12h-2.5M5.25 12H2.75M18.54 5.46l-1.77 1.77M7.23 16.77l-1.77 1.77M18.54 18.54l-1.77-1.77M7.23 7.23L5.46 5.46"></path>
				</svg>
			{:else}
				<svg viewBox="0 0 24 24" class="h-4 w-4 fill-current">
					<path d="M20.76 14.15A8.25 8.25 0 0 1 9.85 3.24a.75.75 0 0 0-.87-.96A9.75 9.75 0 1 0 21.72 15a.75.75 0 0 0-.96-.85Z"></path>
				</svg>
			{/if}
		</span>
		<span class="hidden sm:inline">{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
	</button>

	{@render children()}
</div>
