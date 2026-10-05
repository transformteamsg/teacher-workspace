import assert from 'node:assert/strict';
import {
  createHash,
  createPrivateKey,
  generateKeyPairSync,
  type KeyObject,
  randomUUID,
  X509Certificate,
} from 'node:crypto';
import { createServer, type Server } from 'node:http';
import { after, before, describe, it } from 'node:test';

import { type JSONWebKeySet, type JWTPayload, type ProtectedHeaderParameters, SignJWT } from 'jose';
import { generate } from 'selfsigned';

import { createApp } from '../src/app.ts';
import { createProvider } from '../src/provider.ts';
import { generateCodeChallenge, generateCodeVerifier, OidcClient } from './helpers.ts';

describe('GET /.well-known/openid-configuration', () => {
  let server: Server;

  before(async () => {
    const app = createApp(
      createProvider({
        port: 9876,
        url: 'http://localhost:9876',
        tw: {
          id: 'teacher-workspace',
          redirectUri: 'http://localhost:3000/auth/edupass/callback',
          auth: { method: 'client_secret_post', clientSecret: 'teacher-workspace-secret' },
        },
      }),
    );
    server = createServer((req, res) => {
      res.setHeader('Connection', 'close');
      app(req, res);
    }).listen(9876);
    await new Promise<void>((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it('When requested, then the issuer is the configured URL', async () => {
    const discoveryRes = await globalThis.fetch(
      `http://localhost:9876/.well-known/openid-configuration`,
    );
    assert.equal(discoveryRes.status, 200);

    const doc = (await discoveryRes.json()) as { issuer: string };
    assert.equal(doc.issuer, 'http://localhost:9876');
  });

  it('When requested, then it lists the authorization, token, and JWKS endpoints', async () => {
    const discoveryRes = await globalThis.fetch(
      `http://localhost:9876/.well-known/openid-configuration`,
    );
    assert.equal(discoveryRes.status, 200);

    const doc = (await discoveryRes.json()) as {
      authorization_endpoint?: string;
      token_endpoint?: string;
      jwks_uri?: string;
    };
    assert.ok(doc.authorization_endpoint, 'authorization_endpoint should be present');
    assert.ok(doc.token_endpoint, 'token_endpoint should be present');
    assert.ok(doc.jwks_uri, 'jwks_uri should be present');
  });

  it('When forwarded by an https load balancer, then the endpoints use its public URL', async () => {
    const discoveryRes = await globalThis.fetch(
      `http://localhost:9876/.well-known/openid-configuration`,
      { headers: { 'X-Forwarded-Proto': 'https', 'X-Forwarded-Host': 'mock-edupass.example.com' } },
    );
    assert.equal(discoveryRes.status, 200);

    const openIdConfiguration = (await discoveryRes.json()) as {
      authorization_endpoint: string;
      token_endpoint: string;
      jwks_uri: string;
    };
    assert.equal(
      openIdConfiguration.authorization_endpoint,
      'https://mock-edupass.example.com/authorize',
    );
    assert.equal(openIdConfiguration.token_endpoint, 'https://mock-edupass.example.com/token');
    assert.equal(openIdConfiguration.jwks_uri, 'https://mock-edupass.example.com/jwks');
  });

  it('When requested, then it supports the query response mode', async () => {
    const discoveryRes = await globalThis.fetch(
      `http://localhost:9876/.well-known/openid-configuration`,
    );
    assert.equal(discoveryRes.status, 200);

    const doc = (await discoveryRes.json()) as { response_modes_supported: string[] };
    assert.ok(
      doc.response_modes_supported.includes('query'),
      'response_modes_supported should include query',
    );
  });

  it('When requested, then it supports S256 PKCE', async () => {
    const discoveryRes = await globalThis.fetch(
      `http://localhost:9876/.well-known/openid-configuration`,
    );
    assert.equal(discoveryRes.status, 200);

    const doc = (await discoveryRes.json()) as { code_challenge_methods_supported: string[] };
    assert.ok(
      doc.code_challenge_methods_supported.includes('S256'),
      'code_challenge_methods_supported should include S256',
    );
  });
});

describe('GET /jwks', () => {
  let server: Server;

  before(async () => {
    const app = createApp(
      createProvider({
        port: 9876,
        url: 'http://localhost:9876',
        tw: {
          id: 'teacher-workspace',
          redirectUri: 'http://localhost:3000/auth/edupass/callback',
          auth: { method: 'client_secret_post', clientSecret: 'teacher-workspace-secret' },
        },
      }),
    );
    server = createServer((req, res) => {
      res.setHeader('Connection', 'close');
      app(req, res);
    }).listen(9876);
    await new Promise<void>((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it('When the server restarts, then it publishes a different signing key', async () => {
    const firstJwksRes = await globalThis.fetch(`http://localhost:9876/jwks`);
    const {
      keys: [firstKey],
    } = (await firstJwksRes.json()) as JSONWebKeySet;

    await new Promise<void>((resolve) => server.close(() => resolve()));
    const app = createApp(
      createProvider({
        port: 9876,
        url: 'http://localhost:9876',
        tw: {
          id: 'teacher-workspace',
          redirectUri: 'http://localhost:3000/auth/edupass/callback',
          auth: { method: 'client_secret_post', clientSecret: 'teacher-workspace-secret' },
        },
      }),
    );
    server = createServer((req, res) => {
      res.setHeader('Connection', 'close');
      app(req, res);
    }).listen(9876);
    await new Promise<void>((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });

    const secondJwksRes = await globalThis.fetch(`http://localhost:9876/jwks`);
    const {
      keys: [secondKey],
    } = (await secondJwksRes.json()) as JSONWebKeySet;

    assert.ok(firstKey.kid, 'first server start should publish a kid');
    assert.ok(secondKey.kid, 'restarted server should publish a kid');
    assert.notEqual(firstKey.kid, secondKey.kid);
  });
});

describe('GET /authorize', () => {
  let server: Server;

  before(async () => {
    const app = createApp(
      createProvider({
        port: 9876,
        url: 'http://localhost:9876',
        tw: {
          id: 'teacher-workspace',
          redirectUri: 'http://localhost:3000/auth/edupass/callback',
          auth: { method: 'client_secret_post', clientSecret: 'teacher-workspace-secret' },
        },
      }),
    );
    server = createServer((req, res) => {
      res.setHeader('Connection', 'close');
      app(req, res);
    }).listen(9876);
    await new Promise<void>((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it('When authorization succeeds, then it redirects to the registered redirect URI', async () => {
    const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/auth/edupass/callback',
        response_type: 'code',
        scope: 'openid',
        code_challenge: generateCodeChallenge(generateCodeVerifier()),
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
    );

    assert.equal(authorizeRes.status, 303);
    const redirect = new URL(authorizeRes.headers.get('location') ?? '');
    assert.equal(
      `${redirect.origin}${redirect.pathname}`,
      'http://localhost:3000/auth/edupass/callback',
    );
  });

  it('When authorization succeeds, then the redirect includes an authorization code', async () => {
    const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/auth/edupass/callback',
        response_type: 'code',
        scope: 'openid',
        code_challenge: generateCodeChallenge(generateCodeVerifier()),
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
    );

    assert.equal(authorizeRes.status, 303);
    const redirect = new URL(authorizeRes.headers.get('location') ?? '');
    assert.ok(redirect.searchParams.get('code'), 'redirect should contain code');
  });

  it('When authorization succeeds, then the redirect echoes the state', async () => {
    const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/auth/edupass/callback',
        response_type: 'code',
        scope: 'openid',
        code_challenge: generateCodeChallenge(generateCodeVerifier()),
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
    );

    assert.equal(authorizeRes.status, 303);
    const redirect = new URL(authorizeRes.headers.get('location') ?? '');
    assert.equal(redirect.searchParams.get('state'), 'test-state');
  });

  it('When the redirect URI is not registered, then it does not redirect', async () => {
    const authorizeRes = await globalThis.fetch(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/not-registered',
        response_type: 'code',
        scope: 'openid',
        code_challenge: generateCodeChallenge(generateCodeVerifier()),
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
      { redirect: 'manual' },
    );

    assert.equal(authorizeRes.status, 400);
    assert.equal(authorizeRes.headers.get('location'), null, 'should not redirect');
  });

  it('When the redirect URI is not registered, then it reports invalid_redirect_uri', async () => {
    const authorizeRes = await globalThis.fetch(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/not-registered',
        response_type: 'code',
        scope: 'openid',
        code_challenge: generateCodeChallenge(generateCodeVerifier()),
        code_challenge_method: 'S256',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
      { redirect: 'manual' },
    );

    assert.equal(authorizeRes.status, 400);
    assert.match(await authorizeRes.text(), /invalid_redirect_uri/);
  });

  it('When PKCE is missing, then it returns invalid_request', async () => {
    const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
      `http://localhost:9876/authorize?${new URLSearchParams({
        client_id: 'teacher-workspace',
        redirect_uri: 'http://localhost:3000/auth/edupass/callback',
        response_type: 'code',
        scope: 'openid',
        state: 'test-state',
        nonce: 'test-nonce',
      }).toString()}`,
    );
    const redirect = new URL(authorizeRes.headers.get('location') ?? '');
    assert.equal(redirect.searchParams.get('error'), 'invalid_request');
    assert.equal(redirect.searchParams.get('code'), null, 'no authorization code should be issued');
  });
});

describe('POST /token', () => {
  describe('client_secret_post', () => {
    let server: Server;

    before(async () => {
      const app = createApp(
        createProvider({
          port: 9876,
          url: 'http://localhost:9876',
          tw: {
            id: 'teacher-workspace',
            redirectUri: 'http://localhost:3000/auth/edupass/callback',
            auth: { method: 'client_secret_post', clientSecret: 'teacher-workspace-secret' },
          },
        }),
      );
      server = createServer((req, res) => {
        res.setHeader('Connection', 'close');
        app(req, res);
      }).listen(9876);
      await new Promise<void>((resolve, reject) => {
        server.once('listening', resolve);
        server.once('error', reject);
      });
    });

    after(async () => {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    });

    it('When the code is exchanged, then it returns an ID token', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const tokenBody = (await tokenRes.json()) as Record<string, unknown>;
      assert.ok(tokenBody.id_token, 'response should contain id_token');
    });

    it('When the code is exchanged, then it returns a Bearer access token', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const tokenBody = (await tokenRes.json()) as Record<string, unknown>;
      assert.ok(tokenBody.access_token, 'response should contain access_token');
      assert.equal(tokenBody.token_type, 'Bearer');
    });

    it('When staff-1 signs in, then the ID token identifies them', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );
      assert.equal(claims.sub, 'staff-1');
      assert.equal(claims.email, 'john.smith@example.com');
      assert.equal(claims.name, 'John Smith');
    });

    it('When staff-1 signs in, then the ID token carries their groups', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );
      assert.equal(claims.sub, 'staff-1');
      assert.deepEqual(claims.groups, ['0001_TW_ROLE_TEACHER', '0001_TW_ATTR_PG_ADMIN']);
    });

    it('When the ID token is issued, then its issuer is the configured URL', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );
      assert.equal(claims.iss, 'http://localhost:9876');
    });

    it('When the ID token is issued, then it carries the nonce from the request', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );
      assert.equal(claims.nonce, 'test-nonce');
    });

    it('When the ID token is issued, then it is signed with RS256', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const [encodedHeader] = idToken.split('.');
      const header: ProtectedHeaderParameters = JSON.parse(
        Buffer.from(encodedHeader, 'base64url').toString(),
      );
      assert.equal(header.alg, 'RS256');
    });

    it('When the ID token is issued, then its signature verifies against a key in /jwks', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 200);
      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const [encodedHeader, encodedPayload, signature] = idToken.split('.');
      const header: ProtectedHeaderParameters = JSON.parse(
        Buffer.from(encodedHeader, 'base64url').toString(),
      );

      const jwksRes = await globalThis.fetch(`http://localhost:9876/jwks`);
      const { keys } = (await jwksRes.json()) as JSONWebKeySet;
      const signingKey = keys.find((k) => k.kid === header.kid);
      assert.ok(signingKey, 'JWKS should contain the signing key');

      const publicKey = await crypto.subtle.importKey(
        'jwk',
        signingKey,
        { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' },
        false,
        ['verify'],
      );
      const valid = await crypto.subtle.verify(
        'RSASSA-PKCS1-v1_5',
        publicKey,
        Buffer.from(signature, 'base64url'),
        new TextEncoder().encode(`${encodedHeader}.${encodedPayload}`),
      );
      assert.ok(valid, 'ID token signature should verify against JWKS');
    });

    const groupsFixtures: { account: string; title: string; groups: string[] }[] = [
      { account: 'staff-2', title: 'location-specific role', groups: ['1234_TW_ROLE_TEACHER'] },
      {
        account: 'staff-3',
        title: 'global role and attribute',
        groups: ['X_TW_ROLE_TEACHER', 'X_TW_ATTR_PG_ADMIN'],
      },
      {
        account: 'staff-4',
        title: 'conflict roles at same location',
        groups: ['0001_TW_ROLE_TEACHER', '0001_TW_ROLE_HOD'],
      },
      { account: 'staff-5', title: 'pre-prod TWSTG codes', groups: ['0001_TWSTG_ROLE_TEACHER'] },
      {
        account: 'staff-6',
        title: 'roles across multiple schools',
        groups: ['0001_TW_ROLE_TEACHER', '1001_TW_ROLE_HOD'],
      },
      {
        account: 'staff-7',
        title: 'non-TW role alongside TW role',
        groups: ['X_TW_ROLE_TEACHER', 'X_XX_ROLE_COUNSELLOR'],
      },
    ];

    for (const { account, title, groups } of groupsFixtures) {
      it(`When ${account} signs in, then groups include ${title}`, async () => {
        const codeVerifier = generateCodeVerifier();
        const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
          `http://localhost:9876/authorize?${new URLSearchParams({
            client_id: 'teacher-workspace',
            redirect_uri: 'http://localhost:3000/auth/edupass/callback',
            response_type: 'code',
            scope: 'openid',
            code_challenge: generateCodeChallenge(codeVerifier),
            code_challenge_method: 'S256',
            state: 'test-state',
            nonce: 'test-nonce',
            account,
          }).toString()}`,
        );
        const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
        assert.ok(code, 'Expected an authorization code in the redirect');

        const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
          method: 'POST',
          body: new URLSearchParams({
            grant_type: 'authorization_code',
            code,
            redirect_uri: 'http://localhost:3000/auth/edupass/callback',
            client_id: 'teacher-workspace',
            code_verifier: codeVerifier,
            client_secret: 'teacher-workspace-secret',
          }),
        });
        assert.equal(tokenRes.status, 200);

        const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
        const claims: JWTPayload = JSON.parse(
          Buffer.from(idToken.split('.')[1], 'base64url').toString(),
        );

        assert.equal(claims.sub, account);
        assert.deepEqual(claims.groups, groups);
      });
    }

    it('When staff-8 signs in, then groups is empty', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
          account: 'staff-8',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });
      assert.equal(tokenRes.status, 200);

      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );

      assert.equal(claims.sub, 'staff-8');
      assert.deepEqual(claims.groups, [], 'groups claim should be an empty array');
    });

    it('When staff-8 signs in, then the name claim is absent', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
          account: 'staff-8',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });
      assert.equal(tokenRes.status, 200);

      const { id_token: idToken } = (await tokenRes.json()) as { id_token: string };
      const claims: JWTPayload = JSON.parse(
        Buffer.from(idToken.split('.')[1], 'base64url').toString(),
      );

      assert.equal(claims.sub, 'staff-8');
      assert.equal(
        Object.hasOwn(claims, 'name'),
        false,
        'name claim should be absent, not null or empty',
      );
    });

    it('When the code_verifier is wrong, then it returns 400 invalid_grant', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const wrongVerifier = generateCodeVerifier();
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: wrongVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 400);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_grant');
    });

    it('When the client_secret is wrong, then it returns 401 invalid_client', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'wrong-secret',
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When the code is reused, then it returns 400 invalid_grant', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');

      const firstExchangeRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });
      assert.equal(firstExchangeRes.status, 200);

      const reusedCodeRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });
      assert.equal(reusedCodeRes.status, 400);
      const body = (await reusedCodeRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_grant');
    });

    it('When a client assertion is sent, then it returns 401 invalid_client', async () => {
      const { privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });

      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT' })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });
  });

  describe('private_key_jwt', () => {
    let server: Server;
    let privateKey: KeyObject;
    let otherKey: KeyObject;
    let thumbprint: string;
    let otherThumbprint: string;

    before(async () => {
      const client = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
        keyType: 'rsa',
        keySize: 2048,
        algorithm: 'sha256',
      });
      const other = await generate([{ name: 'commonName', value: 'teacher-workspace' }], {
        keyType: 'rsa',
        keySize: 2048,
        algorithm: 'sha256',
      });

      const certificate = new X509Certificate(client.cert);
      privateKey = createPrivateKey(client.private);
      otherKey = createPrivateKey(other.private);
      thumbprint = createHash('sha256').update(certificate.raw).digest('base64url');
      otherThumbprint = createHash('sha256')
        .update(new X509Certificate(other.cert).raw)
        .digest('base64url');

      const app = createApp(
        createProvider({
          port: 9876,
          url: 'http://localhost:9876',
          tw: {
            id: 'teacher-workspace',
            redirectUri: 'http://localhost:3000/auth/edupass/callback',
            auth: { method: 'private_key_jwt', certificate },
          },
        }),
      );
      server = createServer((req, res) => {
        res.setHeader('Connection', 'close');
        app(req, res);
      }).listen(9876);
      await new Promise<void>((resolve, reject) => {
        server.once('listening', resolve);
        server.once('error', reject);
      });
    });

    after(async () => {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    });

    it('When the client assertion is valid, then it returns an ID token', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 200);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.ok(body.id_token, 'response should contain id_token');
    });

    it('When the client assertion has no x5t#S256, then it returns 401 invalid_client', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT' })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When the x5t#S256 belongs to another certificate, then it returns 401 invalid_client', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': otherThumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When the client assertion is signed by another key, then it returns 401 invalid_client', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(otherKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When the client assertion has expired, then it returns 401 invalid_client', async () => {
      const issuedAt = Math.floor(Date.now() / 1000) - 3600;
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt(issuedAt)
        .setExpirationTime(issuedAt + 300)
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When the client assertion is not signed with PS256, then it returns 401 invalid_client', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'RS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When a client secret is sent, then it returns 401 invalid_client', async () => {
      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_secret: 'teacher-workspace-secret',
        }),
      });

      assert.equal(tokenRes.status, 401);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_client');
    });

    it('When client_assertion_type is missing, then it returns 400 invalid_request', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 400);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_request');
    });

    it('When client_assertion_type is wrong, then it returns 400 invalid_request', async () => {
      const clientAssertion = await new SignJWT()
        .setProtectedHeader({ alg: 'PS256', typ: 'JWT', 'x5t#S256': thumbprint })
        .setIssuer('teacher-workspace')
        .setSubject('teacher-workspace')
        .setAudience(`http://localhost:9876/token`)
        .setJti(randomUUID())
        .setIssuedAt()
        .setExpirationTime('5m')
        .sign(privateKey);

      const codeVerifier = generateCodeVerifier();
      const authorizeRes = await new OidcClient('http://localhost:9876').followRedirects(
        `http://localhost:9876/authorize?${new URLSearchParams({
          client_id: 'teacher-workspace',
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          response_type: 'code',
          scope: 'openid',
          code_challenge: generateCodeChallenge(codeVerifier),
          code_challenge_method: 'S256',
          state: 'test-state',
          nonce: 'test-nonce',
        }).toString()}`,
      );
      const code = new URL(authorizeRes.headers.get('location') ?? '').searchParams.get('code');
      assert.ok(code, 'Expected an authorization code in the redirect');
      const tokenRes = await globalThis.fetch(`http://localhost:9876/token`, {
        method: 'POST',
        body: new URLSearchParams({
          grant_type: 'authorization_code',
          code,
          redirect_uri: 'http://localhost:3000/auth/edupass/callback',
          client_id: 'teacher-workspace',
          code_verifier: codeVerifier,
          client_assertion_type: 'urn:ietf:params:oauth:client-assertion-type:saml2-bearer',
          client_assertion: clientAssertion,
        }),
      });

      assert.equal(tokenRes.status, 400);
      const body = (await tokenRes.json()) as Record<string, unknown>;
      assert.equal(body.error, 'invalid_request');
    });
  });
});
