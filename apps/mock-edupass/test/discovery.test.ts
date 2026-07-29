import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import { discoveryUrl, getJson } from './helpers/client.ts';

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
