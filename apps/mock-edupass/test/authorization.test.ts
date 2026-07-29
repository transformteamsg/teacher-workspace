import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import { REDIRECT_URI } from '../src/provider.ts';
import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import {
  CookieJar,
  authorizeUrl,
  follow,
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
      const uid = /action="\/interaction\/([^/"]+)\/login"/.exec(await picker.text())?.[1];
      assert.ok(uid !== undefined, 'want: an interaction uid; got: none');

      // Walk the redirect chain by hand so every intermediate URL can be inspected.
      const visited: string[] = [];
      let current = `${provider.url}/interaction/${uid}/login`;
      let res = await fetch(current, {
        method: 'POST',
        headers: {
          cookie: jar.header(),
          'content-type': 'application/x-www-form-urlencoded',
        },
        body: new URLSearchParams({ sub: 'teacher-001' }).toString(),
        redirect: 'manual',
      });
      jar.store(res);

      for (let hop = 0; hop < 10 && res.status >= 300 && res.status < 400; hop += 1) {
        const location = res.headers.get('location');
        if (location === null) {
          break;
        }
        current = new URL(location, current).toString();
        visited.push(current);
        res = await fetch(current, { headers: { cookie: jar.header() }, redirect: 'manual' });
        jar.store(res);
      }

      for (const url of visited) {
        for (const param of ['code=', 'state=', 'id_token=', 'access_token='] as const) {
          assert.ok(!url.includes(param), `want URL: free of ${param}; got: ${url}`);
        }
      }
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
