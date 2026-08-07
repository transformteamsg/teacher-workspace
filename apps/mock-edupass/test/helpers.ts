import { randomBytes, createHash } from 'node:crypto';

// --- PKCE utilities ---

export function generateCodeVerifier(): string {
  return randomBytes(32).toString('base64url');
}

export function generateCodeChallenge(verifier: string): string {
  return createHash('sha256').update(verifier).digest('base64url');
}

// --- Cookie-aware HTTP client for OIDC flows ---

export class OidcClient {
  private cookies = new Map<string, string>();
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  async fetch(url: string | URL, init?: RequestInit): Promise<Response> {
    const cookieHeader = [...this.cookies.entries()].map(([k, v]) => `${k}=${v}`).join('; ');

    const headers = new Headers(init?.headers);
    if (cookieHeader) headers.set('Cookie', cookieHeader);

    const res = await globalThis.fetch(url, {
      ...init,
      headers,
      redirect: 'manual',
    });

    for (const setCookie of res.headers.getSetCookie()) {
      const [nameValue] = setCookie.split(';');
      const eqIndex = nameValue.indexOf('=');
      if (eqIndex === -1) continue;
      const name = nameValue.slice(0, eqIndex).trim();
      const value = nameValue.slice(eqIndex + 1).trim();
      this.cookies.set(name, value);
    }

    return res;
  }

  async followRedirects(url: string, init?: RequestInit): Promise<Response> {
    let currentUrl = url;
    let response!: Response;

    for (let i = 0; i < 20; i++) {
      response = await this.fetch(currentUrl, init);
      if (![301, 302, 303, 307, 308].includes(response.status)) break;

      const location = response.headers.get('location');
      if (!location) break;

      currentUrl = new URL(location, currentUrl).href;
      if (!currentUrl.startsWith(this.baseUrl)) break;

      init = { method: 'GET' };
    }

    return response;
  }
}

// --- Form post HTML parser ---

export function parseFormPost(html: string): {
  action: string;
  params: Record<string, string>;
} {
  const actionMatch = html.match(/action="([^"]+)"/);
  const action = actionMatch?.[1] ?? '';

  const params: Record<string, string> = {};
  const inputRegex = /name="([^"]+)"\s+value="([^"]*)"/g;
  let match;
  while ((match = inputRegex.exec(html)) !== null) {
    params[match[1]] = match[2];
  }

  return { action, params };
}
