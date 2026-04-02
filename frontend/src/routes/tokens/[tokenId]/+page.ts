import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import type { RequestListResponse, RequestResponse, TokenResponse } from '$lib/types';

type APIError = {
	error?: string;
};

export const prerender = false;

function parsePositiveInt(value: string | null, fallback: number) {
	if (!value) {
		return fallback;
	}

	const parsed = Number.parseInt(value, 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

export const load: PageLoad = async ({ fetch, params, url }) => {
	const response = await fetch(`/api/tokens/${params.tokenId}`);

	if (!response.ok) {
		const payload = (await response.json().catch(() => null)) as APIError | null;

		throw error(response.status, payload?.error ?? 'Failed to load token');
	}

	const page = parsePositiveInt(url.searchParams.get('page'), 1);
	const filterParams = new URLSearchParams();
	filterParams.set('page', String(page));
	filterParams.set('per_page', '12');
	for (const key of ['method', 'ip', 'search', 'since', 'until']) {
		const val = url.searchParams.get(key);
		if (val) filterParams.set(key, val);
	}
	const requestsResponse = await fetch(
		`/api/tokens/${params.tokenId}/requests?${filterParams.toString()}`
	);

	if (!requestsResponse.ok) {
		const payload = (await requestsResponse.json().catch(() => null)) as APIError | null;

		throw error(requestsResponse.status, payload?.error ?? 'Failed to load requests');
	}

	const requestList = (await requestsResponse.json()) as RequestListResponse;
	const requestedRequestId = url.searchParams.get('request');
	const selectedRequestId = requestedRequestId ?? requestList.data[0]?.uuid ?? null;
	let selectedRequest: RequestResponse | null = null;

	if (selectedRequestId) {
		const requestResponse = await fetch(`/api/tokens/${params.tokenId}/requests/${selectedRequestId}`);

		if (!requestResponse.ok) {
			const payload = (await requestResponse.json().catch(() => null)) as APIError | null;

			throw error(requestResponse.status, payload?.error ?? 'Failed to load request');
		}

		selectedRequest = (await requestResponse.json()) as RequestResponse;
	}

	return {
		token: (await response.json()) as TokenResponse,
		requestList,
		selectedRequestId,
		selectedRequest
	};
};
