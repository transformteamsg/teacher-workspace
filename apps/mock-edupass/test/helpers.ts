import { constants, createHash, createPublicKey, randomBytes, sign } from 'node:crypto';
import { mkdtempSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { createKeyPair } from '../lib/certificate.ts';

export const TEST_PORT = 9876;
export const BASE_URL = `http://localhost:${TEST_PORT}`;
export const CLIENT_ID = 'teacher-workspace';

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

/**
 * Generates an RSA pair and a self-signed certificate for it in a fresh temp directory.
 *
 * A new directory and a new pair are minted per call, so no test reads or overwrites the shared
 * `.certs` pair at the repository root.
 *
 * @returns Both PEM halves, the certificate PEM, and the temp directory holding the files.
 * @throws When generation, the temp directory, or either write fails.
 */
export async function generateClientKeyPair(): Promise<{
  privateKeyPem: string;
  publicKeyPem: string;
  certificatePem: string;
  dir: string;
}> {
  const dir = mkdtempSync(join(tmpdir(), 'mock-edupass-'));
  const { privateKeyPath, certificatePath } = await createKeyPair(dir);
  const privateKeyPem = readFileSync(privateKeyPath, 'utf8');

  return {
    privateKeyPem,
    publicKeyPem: createPublicKey(privateKeyPem).export({ type: 'spki', format: 'pem' }).toString(),
    certificatePem: readFileSync(certificatePath, 'utf8'),
    dir,
  };
}

export const CLIENT_ASSERTION_TYPE = 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer';

/**
 * Signs a compact JWS for the token endpoint's `client_assertion`.
 *
 * PS256 signs with a digest-length salt, which is what RFC 7518 section 3.5 requires and what
 * golang-jwt's `PSSSaltLengthEqualsHash` produces. Setting `alg` to RS256 in `header` switches
 * the signature to PKCS#1 v1.5 padding.
 *
 * @param privateKeyPem - PKCS#8 PEM of the client's RSA private key.
 * @param claims - Overrides merged over the default payload.
 * @param header - Overrides merged over the default JOSE header.
 * @returns The compact serialization, `header.payload.signature`.
 * @throws When the PEM cannot be parsed, or when the key cannot sign.
 */
export function signClientAssertion(
  privateKeyPem: string,
  claims: Record<string, unknown> = {},
  header: Record<string, unknown> = {},
): string {
  const now = Math.floor(Date.now() / 1000);

  const joseHeader = { alg: 'PS256', typ: 'JWT', ...header };
  const payload = {
    iss: CLIENT_ID,
    sub: CLIENT_ID,
    aud: `${BASE_URL}/token`,
    jti: randomBytes(16).toString('base64url'),
    nbf: now,
    iat: now,
    exp: now + 300,
    ...claims,
  };

  const signingInput = `${encodeSegment(joseHeader)}.${encodeSegment(payload)}`;
  const signature = sign('sha256', Buffer.from(signingInput), {
    key: privateKeyPem,
    padding:
      joseHeader.alg === 'RS256' ? constants.RSA_PKCS1_PADDING : constants.RSA_PKCS1_PSS_PADDING,
    saltLength: constants.RSA_PSS_SALTLEN_DIGEST,
  });

  return `${signingInput}.${signature.toString('base64url')}`;
}

function encodeSegment(value: Record<string, unknown>): string {
  return Buffer.from(JSON.stringify(value)).toString('base64url');
}
