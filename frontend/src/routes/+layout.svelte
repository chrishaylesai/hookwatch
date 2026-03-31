<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';

	let { children } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>HookWatch</title>
	<meta
		name="description"
		content="A modern webhook inspector for capturing, replaying, and debugging HTTP traffic."
	/>
	<meta name="theme-color" content="#f4efe4" />
</svelte:head>

<div class="min-h-screen">
	{@render children()}
</div>
