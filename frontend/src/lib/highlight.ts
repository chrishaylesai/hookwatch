type HighlightLanguage = 'json' | 'xml' | 'plain';

type HighlightResult = {
	html: string;
	language: HighlightLanguage;
};

function escapeHtml(value: string): string {
	return value
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;');
}

function normalizeHeaderValue(value: unknown): string {
	if (Array.isArray(value)) {
		return value.map((item) => String(item)).join(', ');
	}

	return typeof value === 'string' ? value : '';
}

function findContentType(headers: Record<string, unknown>): string {
	for (const [key, value] of Object.entries(headers)) {
		if (key.toLowerCase() === 'content-type') {
			return normalizeHeaderValue(value).toLowerCase();
		}
	}

	return '';
}

function looksLikeXML(content: string): boolean {
	const trimmed = content.trim();
	return trimmed.startsWith('<') && trimmed.endsWith('>');
}

function detectLanguage(content: string, headers: Record<string, unknown>): HighlightLanguage {
	const contentType = findContentType(headers);
	const trimmed = content.trim();

	if (
		contentType.includes('/json') ||
		contentType.includes('+json') ||
		trimmed.startsWith('{') ||
		trimmed.startsWith('[')
	) {
		return 'json';
	}

	if (contentType.includes('/xml') || contentType.includes('+xml') || looksLikeXML(trimmed)) {
		return 'xml';
	}

	return 'plain';
}

function prettyPrintJSON(content: string): string {
	try {
		return JSON.stringify(JSON.parse(content), null, 2);
	} catch {
		return content;
	}
}

function prettyPrintXML(content: string): string {
	const normalized = content
		.replaceAll(/>\s+</g, '><')
		.replaceAll(/(>)(<)(\/*)/g, '$1\n$2$3')
		.trim();

	const lines = normalized.split('\n');
	let depth = 0;

	return lines
		.map((line) => {
			const trimmed = line.trim();

			if (!trimmed) {
				return '';
			}

			if (trimmed.startsWith('</')) {
				depth = Math.max(0, depth - 1);
			}

			const indented = `${'  '.repeat(depth)}${trimmed}`;

			if (
				trimmed.startsWith('<') &&
				!trimmed.startsWith('</') &&
				!trimmed.endsWith('/>') &&
				!trimmed.startsWith('<?') &&
				!trimmed.startsWith('<!')
			) {
				depth += 1;
			}

			return indented;
		})
		.filter(Boolean)
		.join('\n');
}

function highlightJSON(content: string): string {
	return escapeHtml(prettyPrintJSON(content)).replace(
		/"(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|\btrue\b|\bfalse\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g,
		(token, offset, source) => {
			const nextChar = source.slice(offset + token.length).trimStart()[0];

			if (token.startsWith('"') && nextChar === ':') {
				return `<span class="hw-code-key">${token}</span>`;
			}

			if (token.startsWith('"')) {
				return `<span class="hw-code-string">${token}</span>`;
			}

			if (token === 'true' || token === 'false' || token === 'null') {
				return `<span class="hw-code-keyword">${token}</span>`;
			}

			return `<span class="hw-code-number">${token}</span>`;
		}
	);
}

function highlightXML(content: string): string {
	const escaped = escapeHtml(prettyPrintXML(content));

	return escaped
		.replaceAll(
			/&lt;!--[\s\S]*?--&gt;/g,
			(match) => `<span class="hw-code-comment">${match}</span>`
		)
		.replaceAll(
			/&lt;\?[\s\S]*?\?&gt;/g,
			(match) => `<span class="hw-code-keyword">${match}</span>`
		)
		.replaceAll(
			/&lt;(\/?)([A-Za-z_:][\w:.-]*)([^&]*?)(\/?)&gt;/g,
			(_match, slash, tagName, attrs, selfClose) => {
				const highlightedAttrs = attrs.replaceAll(
					/([A-Za-z_:][\w:.-]*)(=)(&quot;.*?&quot;|'.*?')/g,
					(_attrMatch: string, name: string, equals: string, value: string) =>
						`<span class="hw-code-attr">${name}</span>${equals}<span class="hw-code-string">${value}</span>`
				);

				return `&lt;${slash}<span class="hw-code-tag">${tagName}</span>${highlightedAttrs}${selfClose}&gt;`;
			}
		);
}

export function highlightRequestBody(
	content: string,
	headers: Record<string, unknown>
): HighlightResult {
	const language = detectLanguage(content, headers);

	switch (language) {
		case 'json':
			return { language, html: highlightJSON(content) };
		case 'xml':
			return { language, html: highlightXML(content) };
		default:
			return { language, html: escapeHtml(content) };
	}
}
