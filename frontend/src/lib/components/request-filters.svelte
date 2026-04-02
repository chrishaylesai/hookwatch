<script lang="ts">
	let {
		method = $bindable(''),
		search = $bindable(''),
		ip = $bindable(''),
		since = $bindable(''),
		until = $bindable(''),
		onchange
	}: {
		method: string;
		search: string;
		ip: string;
		since: string;
		until: string;
		onchange: () => void;
	} = $props();

	let searchTimeout: ReturnType<typeof setTimeout> | undefined;

	function handleSearchInput(event: Event) {
		const target = event.target as HTMLInputElement;
		search = target.value;
		clearTimeout(searchTimeout);
		searchTimeout = setTimeout(onchange, 300);
	}

	function handleChange() {
		onchange();
	}
</script>

<div class="space-y-3">
	<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<div>
			<label class="mb-1 block text-[0.65rem] font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
				Search body
			</label>
			<input
				type="text"
				value={search}
				oninput={handleSearchInput}
				placeholder="Search content..."
				class="h-8 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
			/>
		</div>

		<div>
			<label class="mb-1 block text-[0.65rem] font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
				Method
			</label>
			<select
				bind:value={method}
				onchange={handleChange}
				class="h-8 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
			>
				<option value="">All methods</option>
				<option value="GET">GET</option>
				<option value="POST">POST</option>
				<option value="PUT">PUT</option>
				<option value="PATCH">PATCH</option>
				<option value="DELETE">DELETE</option>
				<option value="OPTIONS">OPTIONS</option>
				<option value="HEAD">HEAD</option>
			</select>
		</div>

		<div>
			<label class="mb-1 block text-[0.65rem] font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
				IP address
			</label>
			<input
				type="text"
				bind:value={ip}
				onchange={handleChange}
				placeholder="Filter by IP..."
				class="h-8 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-3 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
			/>
		</div>

		<div>
			<label class="mb-1 block text-[0.65rem] font-semibold uppercase tracking-[0.05em] text-[var(--muted-foreground)]">
				Since
			</label>
			<input
				type="datetime-local"
				bind:value={since}
				onchange={handleChange}
				class="h-8 w-full rounded-md border border-[var(--border)] bg-[var(--background)] px-2 text-sm focus:border-[var(--accent-strong)] focus:outline-none"
			/>
		</div>
	</div>

	{#if search || method || ip || since || until}
		<div class="flex items-center gap-2">
			<span class="text-xs text-[var(--muted-foreground)]">Filters active</span>
			<button
				type="button"
				onclick={() => {
					search = '';
					method = '';
					ip = '';
					since = '';
					until = '';
					onchange();
				}}
				class="text-xs font-semibold text-[var(--accent-strong)] hover:underline"
			>
				Clear all
			</button>
		</div>
	{/if}
</div>
