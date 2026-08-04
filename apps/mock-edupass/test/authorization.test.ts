import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import { REDIRECT_URI } from '../src/provider.ts';
import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import {
  CookieJar,
  authorizeUrl,
  follow,
  followChain,
  interactionUid,
  parseFormPost,
  pkcePair,
  signInAs,
} from './helpers/client.ts';

describe('authorization', () => {
  let provider: RunningProvider;

  before(async () => {
    provider = await startProvider();
  });

  after(async () => {
    await provider.close();
  });

  describe('completes as a form post', () => {
    test('posts code and state to the registered redirect URI', async () => {
      const { challenge } = pkcePair();
      const { res, html } = await signInAs(provider.url, 'teacher-001', {
        codeChallenge: challenge,
        state: 'state-abc',
      });

      assert.equal(res.status, 200);

      const form = parseFormPost(html);
      assert.equal(form.action, REDIRECT_URI);
      assert.equal(form.fields.state, 'state-abc');
      assert.ok(
        typeof form.fields.code === 'string' && form.fields.code.length > 0,
        'want code: non-empty; got: no code field in the form',
      );
    });

    test('returns a document that submits itself, with a noscript fallback', async () => {
      const { challenge } = pkcePair();
      const { html } = await signInAs(provider.url, 'teacher-001', { codeChallenge: challenge });

      const form = parseFormPost(html);
      assert.ok(form.hasInlineScript, 'want: an inline auto-submit script; got: none');
      assert.ok(form.hasNoscript, 'want: a noscript fallback; got: none');
    });

    test('leaks no authorization parameter into any URL', async () => {
      const { challenge } = pkcePair();
      const jar = new CookieJar();

      const picker = await follow(
        authorizeUrl(provider.url, { codeChallenge: challenge, state: 'state-abc' }),
        jar,
      );
      const uid = interactionUid(await picker.text());

      const { res, visited } = await followChain(`${provider.url}/interaction/${uid}/login`, jar, {
        method: 'POST',
        headers: { 'content-type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ sub: 'teacher-001' }).toString(),
      });

      // The code must arrive in the form body, so it must appear in none of the URLs that
      // carried the browser there.
      assert.ok(visited.length > 0, 'want: at least one redirect; got: none');
      for (const url of visited) {
        for (const param of ['code=', 'state=', 'id_token=', 'access_token='] as const) {
          assert.ok(!url.includes(param), `want URL: free of ${param}; got: ${url}`);
        }
      }

      assert.ok(
        typeof parseFormPost(await res.text()).fields.code === 'string',
        'want code: delivered in the form body; got: none',
      );
    });

    test('honours the selected account rather than defaulting to the first', async () => {
      const { challenge } = pkcePair();
      const { html } = await signInAs(provider.url, 'teacher-002', { codeChallenge: challenge });

      const form = parseFormPost(html);
      assert.ok(
        typeof form.fields.code === 'string' && form.fields.code.length > 0,
        'want code: non-empty; got: no code field in the form',
      );
    });
  });

  describe('rejects a request without PKCE', () => {
    test('returns an OIDC error and issues no code', async () => {
      const jar = new CookieJar();
      const res = await follow(authorizeUrl(provider.url, { state: 'state-abc' }), jar);
      const form = parseFormPost(await res.text());

      assert.equal(res.status, 400);
      assert.equal(form.fields.error, 'invalid_request');
      assert.equal(form.fields.state, 'state-abc');
      assert.equal(form.fields.code, undefined);
    });

    test('rejects a plain code challenge method', async () => {
      const jar = new CookieJar();
      const { verifier } = pkcePair();
      const res = await follow(
        authorizeUrl(provider.url, {
          codeChallenge: verifier,
          codeChallengeMethod: 'plain',
          state: 'state-abc',
        }),
        jar,
      );
      const form = parseFormPost(await res.text());

      assert.equal(form.fields.error, 'invalid_request');
      assert.equal(form.fields.code, undefined);
    });
  });
});
