import assert from 'node:assert/strict';
import { createHash, generateKeyPairSync, X509Certificate } from 'node:crypto';
import { rmSync } from 'node:fs';
import type { Server } from 'node:http';
import { after, before, describe, it } from 'node:test';

import { createApp } from '../src/app.ts';
import { clientJwk } from '../src/jwks.ts';
import {
  BASE_URL,
  CLIENT_ASSERTION_TYPE,
  CLIENT_ID,
  generateClientKeyPair,
  generateCodeChallenge,
  generateCodeVerifier,
  OidcClient,
  parseFormPost,
  signClientAssertion,
  TEST_PORT,
} from './helpers.ts';

const REDIRECT_URI = 'http://localhost:3000/auth/edupass/callback';

const keyPair = await generateClientKeyPair();

after(() => {
  rmSync(keyPair.dir, { recursive: true, force: true });
});

const certificateThumbprint = createHash('sha256')
  .update(new X509Certificate(keyPair.certificatePem).raw)
  .digest('base64url');

function clientAssertion(
  claims: Record<string, unknown> = {},
  header: Record<string, unknown> = {},
): string {
  return signClientAssertion(keyPair.privateKeyPem, claims, {
    'x5t#S256': certificateThumbprint,
    ...header,
  });
}

async function obtainAuthorizationCode(
  client: OidcClient,
  account?: string,
): Promise<{ code: string; codeVerifier: string }> {
  const codeVerifier = generateCodeVerifier();
  const codeChallenge = generateCodeChallenge(codeVerifier);

  const params = new URLSearchParams({
    client_id: CLIENT_ID,
    redirect_uri: REDIRECT_URI,
    response_type: 'code',
    scope: 'openid',
    response_mode: 'form_post',
    code_challenge: codeChallenge,
    code_challenge_method: 'S256',
    state: 'test-state',
    nonce: 'test-nonce',
  });
  if (account) params.set('account', account);

  const response = await client.followRedirects(`${BASE_URL}/authorize?${params.toString()}`);
  const html = await response.text();
  const { params: formParams } = parseFormPost(html);

  assert.ok(formParams.code, 'Expected authorization code in form_post');
  return { code: formParams.code, codeVerifier };
}

async function postToken(params: Record<string, string>): Promise<Response> {
  return globalThis.fetch(`${BASE_URL}/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams(params).toString(),
  });
}

async function exchangeCode(
  code: string,
  codeVerifier: string,
  assertion: string = clientAssertion(),
): Promise<Response> {
  return postToken({
    grant_type: 'authorization_code',
    code,
    redirect_uri: REDIRECT_URI,
    client_id: CLIENT_ID,
    client_assertion_type: CLIENT_ASSERTION_TYPE,
    client_assertion: assertion,
    code_verifier: codeVerifier,
  });
}

async function fetchJwks(baseUrl: string): Promise<Record<string, unknown>[]> {
  const res = await globalThis.fetch(`${baseUrl}/jwks`);
  const jwks = (await res.json()) as { keys: Record<string, unknown>[] };
  return jwks.keys;
}

function decodeJwtPart(jwt: string, index: number): Record<string, unknown> {
  return JSON.parse(Buffer.from(jwt.split('.')[index], 'base64url').toString());
}

function decodeJwtHeader(jwt: string) {
  return decodeJwtPart(jwt, 0);
}
function decodeJwtPayload(jwt: string) {
  return decodeJwtPart(jwt, 1);
}

async function importJwk(jwk: Record<string, unknown>): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    'jwk',
    jwk,
    { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' },
    false,
    ['verify'],
  );
}

async function verifyJwtSignature(jwt: string, publicKey: CryptoKey): Promise<boolean> {
  const parts = jwt.split('.');
  const data = new TextEncoder().encode(`${parts[0]}.${parts[1]}`);
  const signature = Buffer.from(parts[2], 'base64url');
  return crypto.subtle.verify('RSASSA-PKCS1-v1_5', publicKey, signature, data);
}

describe('mock-edupass OIDC provider', () => {
  let server: Server;

  before(async () => {
    const { app } = createApp(TEST_PORT, clientJwk(keyPair.certificatePem));
    server = app.listen(TEST_PORT);
    await new Promise<void>((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  describe('discovery and JWKS', () => {
    it('discovery document has required fields', async () => {
      const res = await globalThis.fetch(`${BASE_URL}/.well-known/openid-configuration`);
      assert.equal(res.status, 200);

      const doc = (await res.json()) as Record<string, unknown>;
      assert.equal(doc.issuer, BASE_URL);
      assert.ok(
        (doc.response_modes_supported as string[]).includes('form_post'),
        'response_modes_supported should include form_post',
      );
      assert.ok(
        (doc.code_challenge_methods_supported as string[]).includes('S256'),
        'code_challenge_methods_supported should include S256',
      );
      assert.ok(doc.jwks_uri, 'jwks_uri should be present');
      assert.ok(doc.authorization_endpoint, 'authorization_endpoint should be present');
      assert.ok(doc.token_endpoint, 'token_endpoint should be present');
    });

    it('JWKS serves a generated public key', async () => {
      const discoveryRes = await globalThis.fetch(`${BASE_URL}/.well-known/openid-configuration`);
      const doc = (await discoveryRes.json()) as Record<string, unknown>;

      const jwksRes = await globalThis.fetch(doc.jwks_uri as string);
      assert.equal(jwksRes.status, 200);

      const jwks = (await jwksRes.json()) as {
        keys: Record<string, unknown>[];
      };
      assert.ok(Array.isArray(jwks.keys), 'keys should be an array');
      assert.ok(jwks.keys.length >= 1, 'should have at least one key');

      const key = jwks.keys[0];
      assert.equal(key.kty, 'RSA');
      assert.notEqual(
        key.kid,
        'keystore-CHANGE-ME',
        'the bundled development key must not be used',
      );
    });

    it('generates a different signing key on every boot', async () => {
      const port = TEST_PORT + 1;
      const { app } = createApp(port, clientJwk(keyPair.certificatePem));
      const other = app.listen(port);
      await new Promise<void>((resolve, reject) => {
        other.once('listening', resolve);
        other.once('error', reject);
      });

      try {
        const [first] = await fetchJwks(BASE_URL);
        const [second] = await fetchJwks(`http://localhost:${port}`);

        assert.ok(first.kid, 'first instance should publish a kid');
        assert.notEqual(first.kid, second.kid);
      } finally {
        await new Promise<void>((resolve) => other.close(() => resolve()));
      }
    });
  });

  describe('full OIDC flow', () => {
    it('completes authorization code flow with PKCE and form_post', async () => {
      const client = new OidcClient(BASE_URL);
      const codeVerifier = generateCodeVerifier();
      const codeChallenge = generateCodeChallenge(codeVerifier);

      const params = new URLSearchParams({
        client_id: CLIENT_ID,
        redirect_uri: REDIRECT_URI,
        response_type: 'code',
        scope: 'openid',
        response_mode: 'form_post',
        code_challenge: codeChallenge,
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      });

      const response = await client.followRedirects(`${BASE_URL}/authorize?${params.toString()}`);

      assert.equal(response.status, 200);
      const html = await response.text();
      const { action, params: formParams } = parseFormPost(html);

      assert.equal(action, REDIRECT_URI);
      assert.ok(formParams.code, 'form should contain code');
      assert.equal(formParams.state, 'test-state');

      const tokenRes = await exchangeCode(formParams.code, codeVerifier);
      assert.equal(tokenRes.status, 200);

      const tokenBody = (await tokenRes.json()) as Record<string, unknown>;
      assert.ok(tokenBody.id_token, 'response should contain id_token');
      assert.ok(tokenBody.access_token, 'response should contain access_token');
      assert.equal(tokenBody.token_type, 'Bearer');

      const idToken = tokenBody.id_token as string;
      const claims = decodeJwtPayload(idToken);
      assert.equal(claims.sub, 'teacher-1');
      assert.equal(claims.email, 'jane.doe@example.com');
      assert.equal(claims.name, 'Jane Doe');
      assert.equal(claims.iss, BASE_URL);
      assert.equal(claims.nonce, 'test-nonce');

      // Verify signature against published JWKS
      const header = decodeJwtHeader(idToken);
      assert.equal(header.alg, 'RS256');

      const keys = await fetchJwks(BASE_URL);
      const signingKey = keys.find((k) => k.kid === header.kid);
      assert.ok(signingKey, 'JWKS should contain the signing key');

      const publicKey = await importJwk(signingKey);
      const valid = await verifyJwtSignature(idToken, publicKey);
      assert.ok(valid, 'ID token signature should verify against JWKS');
    });

    it('absent claim is not present in ID token (teacher-3)', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client, 'teacher-3');

      const tokenRes = await exchangeCode(code, codeVerifier);
      assert.equal(tokenRes.status, 200);

      const tokenBody = (await tokenRes.json()) as Record<string, unknown>;
      const claims = decodeJwtPayload(tokenBody.id_token as string);

      assert.equal(claims.sub, 'teacher-3');
      assert.equal(claims.email, 'no-name@example.com');
      assert.equal(
        Object.hasOwn(claims, 'name'),
        false,
        'name claim should be absent, not null or empty',
      );
    });
  });

  describe('error cases', () => {
    it('rejects authorization without PKCE', async () => {
      const client = new OidcClient(BASE_URL);
      const params = new URLSearchParams({
        client_id: CLIENT_ID,
        redirect_uri: REDIRECT_URI,
        response_type: 'code',
        scope: 'openid',
        response_mode: 'form_post',
        state: 'test-state',
        nonce: 'test-nonce',
      });

      const response = await client.followRedirects(`${BASE_URL}/authorize?${params.toString()}`);
      const html = await response.text();
      const { params: formParams } = parseFormPost(html);

      assert.ok(formParams.error, 'response should contain error');
      assert.equal(formParams.error, 'invalid_request');
      assert.ok(!formParams.code, 'no authorization code should be issued');
    });

    it('rejects token exchange with wrong code_verifier', async () => {
      const client = new OidcClient(BASE_URL);
      const { code } = await obtainAuthorizationCode(client);

      const wrongVerifier = generateCodeVerifier();
      const tokenRes = await exchangeCode(code, wrongVerifier);

      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_grant');
    });

    it('rejects token exchange with reused code', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client);

      // First exchange succeeds
      const firstRes = await exchangeCode(code, codeVerifier);
      assert.equal(firstRes.status, 200);

      // Second exchange fails
      const secondRes = await exchangeCode(code, codeVerifier);
      const body = (await secondRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_grant');
    });
  });

  describe('rejects client authentication', () => {
    it('signed with the wrong key', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client);

      const otherKeyPem = generateKeyPairSync('rsa', { modulusLength: 2048 })
        .privateKey.export({ type: 'pkcs8', format: 'pem' })
        .toString();
      const assertion = signClientAssertion(otherKeyPem, {}, { 'x5t#S256': certificateThumbprint });

      const tokenRes = await exchangeCode(code, codeVerifier, assertion);

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('with a client_secret instead of an assertion', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client);

      const tokenRes = await postToken({
        grant_type: 'authorization_code',
        code,
        redirect_uri: REDIRECT_URI,
        client_id: CLIENT_ID,
        client_secret: 'teacher-workspace-secret',
        code_verifier: codeVerifier,
      });

      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('with a wrong x5t#S256 header', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client);

      const assertion = clientAssertion({}, { 'x5t#S256': 'not-the-certificate-thumbprint' });
      const tokenRes = await exchangeCode(code, codeVerifier, assertion);

      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('with an RS256 assertion', async () => {
      const client = new OidcClient(BASE_URL);
      const { code, codeVerifier } = await obtainAuthorizationCode(client);

      const assertion = clientAssertion({}, { alg: 'RS256' });
      const tokenRes = await exchangeCode(code, codeVerifier, assertion);

      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });
  });
});
