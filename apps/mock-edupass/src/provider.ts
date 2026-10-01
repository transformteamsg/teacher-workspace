import { createHash, generateKeyPairSync } from 'node:crypto';

import Provider, { errors } from 'oidc-provider';

import type { Config } from './config.ts';

export interface Account {
  sub: string;
  email?: string;
  name?: string;
  groups: string[];
}

export const accounts: Account[] = [
  {
    sub: 'staff-1',
    email: 'john.smith@example.com',
    name: 'John Smith',
    groups: ['0001_TW_ROLE_TEACHER', '0001_TW_ATTR_PG_ADMIN'],
  },
  {
    sub: 'staff-2',
    email: 'alice.tan@example.com',
    name: 'Alice Tan',
    groups: ['1234_TW_ROLE_TEACHER'],
  },
  {
    sub: 'staff-3',
    email: 'bob.chen@example.com',
    name: 'Bob Chen',
    groups: ['X_TW_ROLE_TEACHER', 'X_TW_ATTR_PG_ADMIN'],
  },
  {
    sub: 'staff-4',
    email: 'carol.lim@example.com',
    name: 'Carol Lim',
    groups: ['0001_TW_ROLE_TEACHER', '0001_TW_ROLE_HOD'],
  },
  {
    sub: 'staff-5',
    email: 'david.ng@example.com',
    name: 'David Ng',
    groups: ['0001_TWSTG_ROLE_TEACHER'],
  },
  {
    sub: 'staff-6',
    email: 'elena.foo@example.com',
    name: 'Elena Foo',
    groups: ['0001_TW_ROLE_TEACHER', '1001_TW_ROLE_HOD'],
  },
  {
    sub: 'staff-7',
    email: 'jane.doe@example.com',
    name: 'Jane Doe',
    groups: ['X_TW_ROLE_TEACHER', 'X_XX_ROLE_COUNSELLOR'],
  },
  {
    sub: 'staff-8',
    email: 'no-name@example.com',
    groups: [],
  },
];

/**
 * Returns the mock Edupass provider.
 *
 * @param config - The configuration of the App and Relying Party.
 * @returns the {@link Provider}.
 */
export function createProvider(config: Config): Provider {
  return new Provider(`http://localhost:${config.port}`, {
    jwks: {
      keys: [
        generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey.export({ format: 'jwk' }),
      ],
    },

    clients: [
      {
        client_id: config.tw.id,
        redirect_uris: [config.tw.redirectUri],
        response_types: ['code'],
        grant_types: ['authorization_code'],
        ...(config.tw.auth.method === 'private_key_jwt'
          ? {
              token_endpoint_auth_method: 'private_key_jwt',
              token_endpoint_auth_signing_alg: 'PS256',
              jwks: { keys: [config.tw.auth.certificate.publicKey.export({ format: 'jwk' })] },
            }
          : {
              token_endpoint_auth_method: 'client_secret_post',
              client_secret: config.tw.auth.clientSecret,
            }),
      },
    ],

    // Runs only for JWT client auth: https://github.com/panva/node-oidc-provider/blob/v9.11.1/docs/README.md#assertjwtclientauthclaimsandheader
    assertJwtClientAuthClaimsAndHeader: async (_ctx, _claims, header) => {
      if (config.tw.auth.method !== 'private_key_jwt') {
        return;
      }

      const thumbprint = createHash('sha256')
        .update(config.tw.auth.certificate.raw)
        .digest('base64url');

      if (header['x5t#S256'] !== thumbprint) {
        throw new errors.InvalidClientAuth(
          'x5t#S256 is missing or does not match the registered certificate',
        );
      }
    },

    claims: {
      openid: ['sub', 'email', 'name', 'groups'],
    },

    extraParams: ['account'],

    pkce: {
      required: () => true,
    },

    routes: {
      authorization: '/authorize',
    },

    features: {
      devInteractions: { enabled: false },
    },

    cookies: {
      keys: ['mock-edupass-cookie-key'],
      short: { secure: false },
      long: { secure: false },
    },

    interactions: {
      url: (_ctx, interaction) => `/interaction/${interaction.uid}`,
    },

    findAccount: async (_ctx, id) => {
      const account = accounts.find((a) => a.sub === id);
      if (!account) return undefined;
      return {
        accountId: id,
        claims: async () => {
          const claims: { sub: string; [key: string]: string | string[] } = { sub: account.sub };
          if (account.email) claims.email = account.email;
          if (account.name) claims.name = account.name;
          // Always emit groups per Edupass spec: the claim is never absent, only empty.
          claims.groups = account.groups;
          return claims;
        },
      };
    },
  });
}
