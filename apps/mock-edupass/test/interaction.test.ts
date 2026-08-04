import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';

import express from 'express';

import { FAKE_ACCOUNTS } from '../src/accounts.ts';
import { createInteractionRouter } from '../src/interactions.ts';
import { startProvider } from '../src/server.ts';
import type { RunningProvider } from '../src/server.ts';
import {
  CookieJar,
  authorizeUrl,
  exchangeCode,
  follow,
  interactionUid,
  parseFormPost,
  pkcePair,
} from './helpers/client.ts';

describe('login interaction', () => {
  let provider: RunningProvider;

  before(async () => {
    provider = await startProvider();
  });

  after(async () => {
    await provider.close();
  });

  /** reachPicker starts an authorization request and stops at the account picker. */
  async function reachPicker(): Promise<{ jar: CookieJar; uid: string; html: string }> {
    const jar = new CookieJar();
    const { challenge } = pkcePair();
    const res = await follow(authorizeUrl(provider.url, { codeChallenge: challenge }), jar);
    const html = await res.text();
    return { jar, uid: interactionUid(html), html };
  }

  test('lists every fake account', async () => {
    const { html } = await reachPicker();

    for (const account of FAKE_ACCOUNTS) {
      assert.ok(
        html.includes(account.sub),
        `want the picker: listing ${account.sub}; got: markup without it`,
      );
      assert.ok(
        html.includes(account.email),
        `want the picker: listing ${account.email}; got: markup without it`,
      );
    }
  });

  test('references no external subresource', async () => {
    const { html } = await reachPicker();

    for (const pattern of [/<link\b/i, /<script\b/i, /<img\b/i, /url\(/i]) {
      assert.ok(
        !pattern.test(html),
        `want the picker: free of ${String(pattern)}; got: markup that loads a subresource`,
      );
    }
  });

  test('rejects an account that is not on offer', async () => {
    const { jar, uid } = await reachPicker();

    const res = await fetch(`${provider.url}/interaction/${uid}/login`, {
      method: 'POST',
      headers: {
        cookie: jar.header(),
        'content-type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({ sub: 'teacher-999' }).toString(),
      redirect: 'manual',
    });

    assert.equal(res.status, 400);
  });

  test('rejects a login with no account submitted', async () => {
    const { jar, uid } = await reachPicker();

    const res = await fetch(`${provider.url}/interaction/${uid}/login`, {
      method: 'POST',
      headers: {
        cookie: jar.header(),
        'content-type': 'application/x-www-form-urlencoded',
      },
      body: '',
      redirect: 'manual',
    });

    assert.equal(res.status, 400);
  });

  test('rejects an interaction that was never issued', async () => {
    const res = await fetch(`${provider.url}/interaction/not-a-real-uid`, { redirect: 'manual' });
    const html = await res.text();

    assert.equal(res.status, 400);
    assert.ok(!html.includes('at Object.'), 'want: no stack trace in the page; got: one');
  });
});

describe('server lifecycle', () => {
  test('releases its port on close', async () => {
    const running = await startProvider();
    const { url } = running;

    assert.equal((await fetch(`${url}/.well-known/openid-configuration`)).status, 200);

    await running.close();

    await assert.rejects(
      fetch(`${url}/.well-known/openid-configuration`),
      'want: a refused connection after close; got: a served response',
    );
  });
});

describe('mount order', () => {
  /**
   * startMiswired reuses the real socket lifecycle and swaps only the handler, so this stays a
   * test of wiring rather than a second copy of the server that can drift from it.
   *
   * The arrangement it builds puts a body parser ahead of the provider, which the real
   * application avoids. oidc-provider tolerates it: when the raw stream has already been
   * drained it falls back to `req.body` and warns that an upstream parser is not recommended.
   * This characterises that fallback, so its removal in a future release shows up here rather
   * than as a broken token endpoint.
   */
  function startMiswired(): Promise<RunningProvider> {
    return startProvider({
      buildHandler: (provider) => {
        const app = express();
        app.use(express.urlencoded({ extended: false }));
        app.use(createInteractionRouter(provider));
        app.use(provider.callback());
        return app;
      },
    });
  }

  test('tolerates an application-wide body parser by falling back to req.body', async () => {
    const running = await startMiswired();

    try {
      const jar = new CookieJar();
      const { verifier, challenge } = pkcePair();

      const picker = await follow(authorizeUrl(running.url, { codeChallenge: challenge }), jar);
      const uid = interactionUid(await picker.text());

      const done = await follow(`${running.url}/interaction/${uid}/login`, jar, {
        method: 'POST',
        headers: { 'content-type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ sub: 'teacher-001' }).toString(),
      });

      const { code } = parseFormPost(await done.text()).fields;
      const res = await exchangeCode(running.url, { code, codeVerifier: verifier });

      assert.equal(
        res.status,
        200,
        'want: the documented req.body fallback to carry the exchange; got: a failure',
      );
    } finally {
      await running.close();
    }
  });
});
