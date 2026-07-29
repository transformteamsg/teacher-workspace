import { createHash, createPublicKey, randomBytes, verify as verifySignature } from 'node:crypto';

import { CLIENT_ID, CLIENT_SECRET, REDIRECT_URI } from '../../src/provider.ts';

/**
 * CookieJar is the smallest thing that keeps a redirect chain working: fetch does not carry
 * cookies between calls, and the provider's session and interaction state live in them.
 * Paths and domains are ignored, which is safe when every request goes to one local origin.
 */
export class CookieJar {
  readonly #cookies = new Map<string, string>();

  store(res: Response): void {
    for (const raw of res.headers.getSetCookie()) {
      const pair = raw.split(';')[0];
      const eq = pair.indexOf('=');
      if (eq === -1) {
        continue;
      }

      const name = pair.slice(0, eq).trim();
      const value = pair.slice(eq + 1).trim();
      // The provider clears a cookie by sending it back empty, notably the interaction
      // cookie once the flow resumes. Keeping the stale value breaks the next request.
      if (value === '') {
        this.#cookies.delete(name);
      } else {
        this.#cookies.set(name, value);
      }
    }
  }

  header(): string {
    return [...this.#cookies].map(([name, value]) => `${name}=${value}`).join('; ');
  }
}

async function request(url: string, jar: CookieJar, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const cookie = jar.header();
  if (cookie !== '') {
    headers.set('cookie', cookie);
  }

  const res = await fetch(url, { ...init, headers, redirect: 'manual' });
  jar.store(res);
  return res;
}

/** follow issues the request and walks Location headers until the response is not a redirect. */
export async function follow(
  url: string,
  jar: CookieJar,
  init: RequestInit = {},
  max = 10,
): Promise<Response> {
  let current = url;
  let res = await request(current, jar, init);

  for (let hop = 0; hop < max; hop += 1) {
    if (res.status < 300 || res.status >= 400) {
      return res;
    }

    const location = res.headers.get('location');
    if (location === null) {
      return res;
    }

    current = new URL(location, current).toString();
    res = await request(current, jar);
  }

  throw new Error(`want: a terminal response; got: more than ${max} redirects`);
}

/** pkcePair returns a fresh verifier and its S256 challenge. */
export function pkcePair(): { verifier: string; challenge: string } {
  const verifier = randomBytes(32).toString('base64url');
  const challenge = createHash('sha256').update(verifier).digest('base64url');
  return { verifier, challenge };
}

export interface AuthorizeOptions {
  clientId?: string;
  redirectUri?: string;
  responseType?: string;
  responseMode?: string;
  scope?: string;
  state?: string;
  nonce?: string;
  /** Omitted from the request entirely when absent, which is how the no-PKCE case is built. */
  codeChallenge?: string;
  codeChallengeMethod?: string;
}

export function authorizeUrl(issuer: string, opts: AuthorizeOptions = {}): string {
  const params = new URLSearchParams({
    client_id: opts.clientId ?? CLIENT_ID,
    redirect_uri: opts.redirectUri ?? REDIRECT_URI,
    response_type: opts.responseType ?? 'code',
    response_mode: opts.responseMode ?? 'form_post',
    scope: opts.scope ?? 'openid',
    state: opts.state ?? 'state-value',
  });

  if (opts.nonce !== undefined) {
    params.set('nonce', opts.nonce);
  }
  if (opts.codeChallenge !== undefined) {
    params.set('code_challenge', opts.codeChallenge);
    params.set('code_challenge_method', opts.codeChallengeMethod ?? 'S256');
  } else if (opts.codeChallengeMethod !== undefined) {
    params.set('code_challenge_method', opts.codeChallengeMethod);
  }

  return `${issuer}/auth?${params.toString()}`;
}

function decodeHtml(value: string): string {
  return value
    .replaceAll('&lt;', '<')
    .replaceAll('&gt;', '>')
    .replaceAll('&quot;', '"')
    .replaceAll('&#39;', "'")
    .replaceAll('&#x27;', "'")
    .replaceAll('&amp;', '&');
}

export interface FormPost {
  action: string;
  fields: Record<string, string>;
  hasInlineScript: boolean;
  hasNoscript: boolean;
}

/** parseFormPost reads the auto-submitting document the provider returns for form_post. */
export function parseFormPost(html: string): FormPost {
  const action = /<form[^>]*action="([^"]*)"/.exec(html)?.[1];
  if (action === undefined) {
    throw new Error('want: a form action; got: no form in the document');
  }

  const fields: Record<string, string> = {};
  for (const match of html.matchAll(/<input[^>]*name="([^"]*)"[^>]*value="([^"]*)"/g)) {
    fields[decodeHtml(match[1])] = decodeHtml(match[2]);
  }

  return {
    action: decodeHtml(action),
    fields,
    hasInlineScript: /<script>[\s\S]*document\.forms\[0\]\.submit/.test(html),
    hasNoscript: html.includes('<noscript>'),
  };
}

/** interactionUid pulls the interaction identifier out of the account picker markup. */
export function interactionUid(html: string): string {
  const uid = /action="\/interaction\/([^/"]+)\/login"/.exec(html)?.[1];
  if (uid === undefined) {
    throw new Error('want: an interaction uid; got: no login form in the document');
  }
  return uid;
}

export async function getJson(url: string): Promise<Record<string, unknown>> {
  const res = await fetch(url);
  return (await res.json()) as Record<string, unknown>;
}

export function discoveryUrl(issuer: string): string {
  return `${issuer}/.well-known/openid-configuration`;
}

/** signInAs drives the whole browser-side flow and returns the final form_post response. */
export async function signInAs(
  issuer: string,
  sub: string,
  opts: AuthorizeOptions = {},
): Promise<{ res: Response; html: string }> {
  const jar = new CookieJar();

  const picker = await follow(authorizeUrl(issuer, opts), jar);
  const uid = interactionUid(await picker.text());

  const res = await follow(`${issuer}/interaction/${uid}/login`, jar, {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ sub }).toString(),
  });

  return { res, html: await res.text() };
}

export interface ExchangeOptions {
  code: string;
  codeVerifier: string;
  clientId?: string;
  clientSecret?: string;
  redirectUri?: string;
}

/** exchangeCode posts to the token endpoint using client_secret_basic. */
export async function exchangeCode(issuer: string, opts: ExchangeOptions): Promise<Response> {
  const id = opts.clientId ?? CLIENT_ID;
  const secret = opts.clientSecret ?? CLIENT_SECRET;
  const basic = Buffer.from(`${id}:${secret}`).toString('base64');

  return fetch(`${issuer}/token`, {
    method: 'POST',
    headers: {
      authorization: `Basic ${basic}`,
      'content-type': 'application/x-www-form-urlencoded',
    },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      code: opts.code,
      redirect_uri: opts.redirectUri ?? REDIRECT_URI,
      code_verifier: opts.codeVerifier,
    }).toString(),
  });
}

export interface DecodedIdToken {
  header: Record<string, unknown>;
  payload: Record<string, unknown>;
}

/**
 * verifyIdToken checks an RS256 ID token against a JWKS document, using node:crypto so the
 * test suite needs no JWT library. It throws when no key matches or the signature is bad.
 */
export function verifyIdToken(idToken: string, jwks: { keys: JsonWebKey[] }): DecodedIdToken {
  const [encodedHeader, encodedPayload, encodedSignature] = idToken.split('.');
  const header = JSON.parse(Buffer.from(encodedHeader, 'base64url').toString()) as Record<
    string,
    unknown
  >;

  const jwk = jwks.keys.find((key) => (key as { kid?: string }).kid === header.kid);
  if (jwk === undefined) {
    throw new Error(`want: a JWKS key for kid ${String(header.kid)}; got: none`);
  }

  const ok = verifySignature(
    'sha256',
    Buffer.from(`${encodedHeader}.${encodedPayload}`),
    createPublicKey({ key: jwk, format: 'jwk' }),
    Buffer.from(encodedSignature, 'base64url'),
  );
  if (!ok) {
    throw new Error('want: a valid ID token signature; got: verification failure');
  }

  return {
    header,
    payload: JSON.parse(Buffer.from(encodedPayload, 'base64url').toString()) as Record<
      string,
      unknown
    >,
  };
}
