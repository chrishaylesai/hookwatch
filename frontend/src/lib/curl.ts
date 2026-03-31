import type { RequestResponse } from '$lib/types';

function shellQuote(value: string): string {
	return `'${value.replaceAll("'", `'\\''`)}'`;
}

function normalizeHeaderValue(value: unknown): string {
	if (Array.isArray(value)) {
		return value.map((item) => String(item)).join(', ');
	}

	if (value === null || value === undefined) {
		return '';
	}

	return String(value);
}

function shouldSkipHeader(name: string): boolean {
	const normalized = name.toLowerCase();
	return normalized === 'host' || normalized === 'content-length';
}

export function requestToCurl(request: RequestResponse): string {
	const parts: string[] = ['curl'];
	const method = request.method.toUpperCase();

	if (method !== 'GET') {
		parts.push('-X', shellQuote(method));
	}

	const headers = Object.entries(request.headers)
		.filter(([name]) => !shouldSkipHeader(name))
		.sort((a, b) => a[0].localeCompare(b[0]));

	for (const [name, value] of headers) {
		const normalized = normalizeHeaderValue(value);
		if (!normalized) {
			continue;
		}

		parts.push('-H', shellQuote(`${name}: ${normalized}`));
	}

	if (request.content) {
		parts.push('--data-binary', shellQuote(request.content));
	}

	parts.push(shellQuote(request.url));

	return parts.join(' ');
}
