export type TokenResponse = {
	uuid: string;
	owner_id?: string;
	receive_mode: string;
	view_mode: string;
	receive_secret?: string;
	receive_secret_prefix?: string;
	default_status: number;
	default_content_type: string;
	default_content: string;
	max_requests: number;
	timeout: number;
	cors: boolean;
	created_at: string;
	updated_at: string;
	expires_at: string;
};

export type RequestResponse = {
	uuid: string;
	token_id: string;
	ip: string;
	hostname: string;
	method: string;
	user_agent: string;
	content: string;
	query: string;
	headers: Record<string, unknown>;
	form_data: Record<string, unknown>;
	url: string;
	created_at: string;
};

export type RequestListResponse = {
	data: RequestResponse[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
};

export type RequestCreatedEvent = {
	request: RequestResponse;
	total: number;
};

export type TokenUpdatedEvent = {
	token: TokenResponse;
};

export type TokenDeletedEvent = {
	token_id: string;
};

export type AuthUser = {
	id: string;
	email: string;
	display_name: string;
	global_role: string;
	created_at: string;
};

export type AuthInfo = {
	auth_mode: string;
};

export type HookGrant = {
	id: string;
	token_id: string;
	user_id: string;
	role: string;
	granted_by: string;
	created_at: string;
};

export type AdminUser = {
	id: string;
	email: string;
	display_name: string;
	global_role: string;
	oidc_provider?: string;
	created_at: string;
	updated_at: string;
};
