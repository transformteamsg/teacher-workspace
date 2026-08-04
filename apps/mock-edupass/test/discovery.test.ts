import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import {
  discoveryUrl,
  exchangeCode,
  getJson,
  parseFormPost,
  pkcePair,
  signInAs,
} from './helpers/client.ts';

describe('discovery', () => {
  let provider: RunningProvider;

  before(async () => {
    provider = await startProvider();
  });

  after(async () => {
    await provider.close();
  });

  test('serves the discovery document without credentials', async () => {
    const res = await fetch(discoveryUrl(provider.url));

    assert.equal(res.status, 200, 'want status: 200; got a non-200 discovery response');

    const doc = (await res.json()) as Record<string, unknown>;
    assert.equal(doc.issuer, provider.url);
  });

  test('advertises form_post as a supported response mode', async () => {
    const doc = await getJson(discoveryUrl(provider.url));
    const modes = doc.response_modes_supported as string[];

    assert.ok(
      modes.includes('form_post'),
      `want response_modes_supported: containing "form_post"; got: ${JSON.stringify(modes)}`,
    );
  });

  test('advertises S256 as a supported code challenge method', async () => {
    const doc = await getJson(discoveryUrl(provider.url));
    const methods = doc.code_challenge_methods_supported as string[];

    assert.ok(
      methods.includes('S256'),
      `want code_challenge_methods_supported: containing "S256"; got: ${JSON.stringify(methods)}`,
    );
  });

  test('serves a signing key at the advertised jwks_uri', async () => {
    const doc = await getJson(discoveryUrl(provider.url));
    const jwks = (await getJson(doc.jwks_uri as string)) as unknown as { keys: JsonWebKey[] };

    assert.ok(jwks.keys.length > 0, 'want keys: non-empty; got: an empty JWKS');
    for (const key of jwks.keys) {
      assert.equal((key as { use?: string }).use, 'sig');
    }
  });

  test('reflects the bound port in every advertised endpoint', async () => {
    const doc = await getJson(discoveryUrl(provider.url));

    for (const field of [
      'issuer',
      'authorization_endpoint',
      'token_endpoint',
      'jwks_uri',
    ] as const) {
      assert.ok(
        (doc[field] as string).startsWith(provider.url),
        `want ${field}: starting with ${provider.url}; got: ${String(doc[field])}`,
      );
    }
  });

  test('defaults the issuer to the bound loopback address', () => {
    assert.equal(provider.url, provider.address);
  });

  test('publishes no private key material', async () => {
    const doc = await getJson(discoveryUrl(provider.url));
    const jwks = (await getJson(doc.jwks_uri as string)) as unknown as { keys: JsonWebKey[] };

    for (const key of jwks.keys) {
      for (const secret of ['d', 'p', 'q', 'dp', 'dq', 'qi'] as const) {
        assert.equal(
          (key as Record<string, unknown>)[secret],
          undefined,
          `want ${secret}: absent from the published JWKS; got: a private component`,
        );
      }
    }
  });
});

describe('configured issuer', () => {
  const ISSUER = 'https://mock-edupass.example';

  let running: RunningProvider;

  before(async () => {
    running = await startProvider({ issuer: ISSUER });
  });

  after(async () => {
    await running.close();
  });

  /**
   * OIDC Discovery requires the advertised issuer to be identical to the URL the document was
   * fetched from, and the library derives the other endpoints from the request Host header.
   * Without a configurable issuer the two disagree the moment the provider is reached under
   * any name other than its bound address, which is every containerised deployment.
   */
  test('advertises the configured issuer rather than the bound address', async () => {
    const doc = await getJson(discoveryUrl(running.address));

    assert.equal(doc.issuer, ISSUER);
    assert.notEqual(running.url, running.address);
  });

  test('signs ID tokens with the configured issuer', async () => {
    const { challenge, verifier } = pkcePair();
    const { html } = await signInAs(running.address, 'teacher-001', { codeChallenge: challenge });
    const { code } = parseFormPost(html).fields;

    const res = await exchangeCode(running.address, { code, codeVerifier: verifier });
    const body = (await res.json()) as { id_token: string };
    const payload = JSON.parse(
      Buffer.from(body.id_token.split('.')[1], 'base64url').toString(),
    ) as Record<string, unknown>;

    assert.equal(payload.iss, ISSUER);
  });
});
