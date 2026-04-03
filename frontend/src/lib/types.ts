export type TokenResponse = {
	uuid: string;
	owner_id?: string;
	receive_mode: string;
	view_mode: string;
	persistent: boolean;
	receive_secret?: string;
	receive_secret_prefix?: string;
	signature_provider?: string;
	signature_secret_configured: boolean;
	default_status: number;
	default_content_type: string;
	default_content: string;
	max_requests: number;
	timeout: number;
	cors: boolean;
	rate_limit: number;
	can_delete?: boolean;
	access_role?: string;
	owner_display?: string;
	created_at: string;
	updated_at: string;
	expires_at: string;
};

export type TokenListResponse = {
	data: TokenResponse[];
	total: number;
	limit: number;
	offset: number;
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
	size: number;
	signature_validation: SignatureValidation;
	created_at: string;
};

export type SignatureValidation = {
	status: 'unknown' | 'valid' | 'invalid';
	provider?: string;
	error?: string;
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

export type Action = {
	uuid: string;
	token_id: string;
	type: 'forward' | 'filter' | 'delay' | 'transform';
	config: ForwardConfig | FilterConfig | DelayConfig | TransformConfig;
	sort_order: number;
	enabled: boolean;
	created_at: string;
	updated_at: string;
};

export type ForwardConfig = {
	url: string;
	method?: string;
	headers?: Record<string, string>;
	timeout?: number;
};

export type FilterConfig = {
	field: string;
	operator: 'equals' | 'contains' | 'matches' | 'exists';
	value?: string;
	negate?: boolean;
};

export type DelayConfig = {
	duration_ms: number;
};

export type TransformConfig = {
	status?: number;
	content_type?: string;
	body?: string;
};

export type ActionLog = {
	uuid: string;
	action_id: string;
	request_id: string;
	status: 'pending' | 'running' | 'success' | 'failed' | 'skipped';
	result: Record<string, unknown>;
	started_at?: string;
	completed_at?: string;
};

export type ActionListResponse = {
	data: Action[];
};

export type ActionCompletedEvent = {
	action_log: ActionLog;
};

export type ReplayResponse = {
	status: number;
	headers: Record<string, string>;
	body: string;
	duration_ms: number;
	url: string;
};

export type RequestDiffSection = {
	key: string;
	label: string;
	format: 'text' | 'json';
	left: string;
	right: string;
	changed: boolean;
};

export type RequestDiffResponse = {
	left_request_id: string;
	right_request_id: string;
	sections: RequestDiffSection[];
};
