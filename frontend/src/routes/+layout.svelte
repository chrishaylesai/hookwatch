<script lang="ts">
	import { browser } from '$app/environment';
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();

	type Theme = 'light' | 'dark';

	const themeStorageKey = 'hookwatch-theme';
	const initialTheme: Theme =
		browser && document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';

	let theme = $state<Theme>(initialTheme);

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
	<button
		type="button"
		class="hw-theme-toggle fixed right-4 bottom-4 z-50 inline-flex items-center gap-2 rounded-full px-3 py-3 text-sm font-medium tracking-[0.01em] transition duration-200 ease-out focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--ring)] sm:right-5 sm:bottom-5 sm:gap-3 sm:px-4"
		aria-label={`Switch to ${themeLabel.toLowerCase()}`}
		title={`Switch to ${themeLabel.toLowerCase()}`}
		onclick={toggleTheme}
	>
		<span
			class="inline-flex h-8 w-8 items-center justify-center rounded-full bg-[var(--accent-soft)] text-[var(--accent-strong)]"
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
