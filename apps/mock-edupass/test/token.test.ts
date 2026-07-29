import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import { CLIENT_ID } from '../src/provider.ts';
import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import {
  discoveryUrl,
  exchangeCode,
  getJson,
  parseFormPost,
  pkcePair,
  signInAs,
  verifyIdToken,
} from './helpers/client.ts';
import type { AuthorizeOptions } from './helpers/client.ts';

describe('token exchange', () => {
  let provider: RunningProvider;
  let jwks: { keys: JsonWebKey[] };

  before(async () => {
    provider = await startProvider();
    const doc = await getJson(discoveryUrl(provider.url));
    jwks = (await getJson(doc.jwks_uri as string)) as unknown as { keys: JsonWebKey[] };
  });

  after(async () => {
    await provider.close();
  });

  /** authorize runs the browser-side flow and returns the code plus its PKCE verifier. */
  async function authorize(
    sub: string,
    opts: AuthorizeOptions = {},
  ): Promise<{ code: string; verifier: string }> {
    const { verifier, challenge } = pkcePair();
    const { html } = await signInAs(provider.url, sub, { codeChallenge: challenge, ...opts });
    return { code: parseFormPost(html).fields.code, verifier };
  }

  describe('issues an ID token for the selected account', () => {
    test('carries sub, email, and name from the openid scope alone', async () => {
      const { code, verifier } = await authorize('teacher-001');
      const res = await exchangeCode(provider.url, { code, codeVerifier: verifier });

      assert.equal(res.status, 200);

      const body = (await res.json()) as { id_token: string; scope: string };
      assert.equal(body.scope, 'openid');

      const { payload } = verifyIdToken(body.id_token, jwks);
      assert.equal(payload.sub, 'teacher-001');
      assert.equal(payload.email, 'amirah.rahman@schools.gov.sg');
      assert.equal(payload.name, 'Amirah Rahman');
    });

    test('omits a claim the account leaves unset', async () => {
      const { code, verifier } = await authorize('teacher-003');
      const res = await exchangeCode(provider.url, { code, codeVerifier: verifier });
      const body = (await res.json()) as { id_token: string };

      const { payload } = verifyIdToken(body.id_token, jwks);
      assert.equal(payload.sub, 'teacher-003');
      assert.equal(payload.email, 'no.name@schools.gov.sg');
      assert.ok(
        !('name' in payload),
        `want name: absent from the ID token; got: ${JSON.stringify(payload.name)}`,
      );
    });

    test('signs with a key published at jwks_uri', async () => {
      const { code, verifier } = await authorize('teacher-002');
      const res = await exchangeCode(provider.url, { code, codeVerifier: verifier });
      const body = (await res.json()) as { id_token: string };

      const { header } = verifyIdToken(body.id_token, jwks);
      const kids = jwks.keys.map((key) => (key as { kid?: string }).kid);
      assert.ok(
        kids.includes(header.kid as string),
        `want kid: one of ${JSON.stringify(kids)}; got: ${String(header.kid)}`,
      );
    });

    test('binds the token to the issuer, audience, and nonce of the request', async () => {
      const { code, verifier } = await authorize('teacher-001', { nonce: 'nonce-abc' });
      const res = await exchangeCode(provider.url, { code, codeVerifier: verifier });
      const body = (await res.json()) as { id_token: string };

      const { payload } = verifyIdToken(body.id_token, jwks);
      assert.equal(payload.iss, provider.url);
      assert.equal(payload.aud, CLIENT_ID);
      assert.equal(payload.nonce, 'nonce-abc');
    });
  });

  describe('rejects an invalid credential', () => {
    test('rejects a mismatched code verifier', async () => {
      const { code } = await authorize('teacher-001');
      const other = pkcePair().verifier;

      const res = await exchangeCode(provider.url, { code, codeVerifier: other });
      const body = (await res.json()) as { error: string };

      assert.equal(res.status, 400);
      assert.equal(body.error, 'invalid_grant');
      assert.equal((body as Record<string, unknown>).id_token, undefined);
    });

    test('rejects an incorrect client secret', async () => {
      const { code, verifier } = await authorize('teacher-001');

      const res = await exchangeCode(provider.url, {
        code,
        codeVerifier: verifier,
        clientSecret: 'not-the-secret',
      });
      const body = (await res.json()) as { error: string };

      assert.equal(res.status, 401);
      assert.equal(body.error, 'invalid_client');
      assert.equal((body as Record<string, unknown>).id_token, undefined);
    });

    test('rejects a code that has already been redeemed', async () => {
      const { code, verifier } = await authorize('teacher-001');

      const first = await exchangeCode(provider.url, { code, codeVerifier: verifier });
      assert.equal(first.status, 200);

      const second = await exchangeCode(provider.url, { code, codeVerifier: verifier });
      const body = (await second.json()) as { error: string };

      assert.equal(second.status, 400);
      assert.equal(body.error, 'invalid_grant');
      assert.equal((body as Record<string, unknown>).id_token, undefined);
    });

    test('rejects a code that was never issued', async () => {
      const res = await exchangeCode(provider.url, {
        code: 'not-a-real-code',
        codeVerifier: pkcePair().verifier,
      });
      const body = (await res.json()) as { error: string };

      assert.equal(res.status, 400);
      assert.equal(body.error, 'invalid_grant');
    });
  });
});
