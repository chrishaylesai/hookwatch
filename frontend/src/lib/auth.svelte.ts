import { browser } from '$app/environment';
import type { AuthInfo, AuthUser } from '$lib/types';

let user = $state<AuthUser | null>(null);
let authMode = $state<string>('none');
let loaded = $state(false);

export function getAuth() {
	return {
		get user() {
			return user;
		},
		get authMode() {
			return authMode;
		},
		get loaded() {
			return loaded;
		},
		get isAuthenticated() {
			return user !== null;
		},
		get isAdmin() {
			return user?.global_role === 'admin';
		},
		get authEnabled() {
			return authMode !== 'none';
		}
	};
}

export async function loadAuth(): Promise<void> {
	if (!browser) return;

	try {
		const infoRes = await fetch('/api/auth/info');
		if (infoRes.ok) {
			const info = (await infoRes.json()) as AuthInfo;
			authMode = info.auth_mode;
		}

		if (authMode !== 'none') {
			const meRes = await fetch('/api/auth/me');
			if (meRes.ok) {
				user = (await meRes.json()) as AuthUser;
			} else {
				user = null;
			}
		}
	} catch {
		// Network error — leave defaults
	} finally {
		loaded = true;
	}
}

export function setUser(u: AuthUser | null) {
	user = u;
}

export async function logout(): Promise<void> {
	try {
		await fetch('/api/auth/logout', { method: 'POST' });
	} catch {
		// Best effort
	}
	user = null;
}
