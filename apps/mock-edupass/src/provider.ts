import { createHash, generateKeyPairSync } from 'node:crypto';

import Provider, { errors } from 'oidc-provider';

import type { Config } from './config.ts';

export interface Account {
  id: string;
  name: string;
  cesEmail?: string;
  groups: string[];
}

export const accounts: Account[] = [
  {
    id: 'staff-1',
    name: 'John Smith',
    cesEmail: 'john.smith@example.com',
    groups: ['0001_TW_ROLE_TEACHER', '0001_TW_ATTR_PG_ADMIN'],
  },
  {
    id: 'staff-2',
    name: 'Alice Tan',
    cesEmail: 'alice.tan@example.com',
    groups: ['1234_TW_ROLE_TEACHER'],
  },
  {
    id: 'staff-3',
    name: 'Bob Chen',
    cesEmail: 'bob.chen@example.com',
    groups: ['X_TW_ROLE_TEACHER', 'X_TW_ATTR_PG_ADMIN'],
  },
  {
    id: 'staff-4',
    name: 'Carol Lim',
    cesEmail: 'carol.lim@example.com',
    groups: ['0001_TW_ROLE_TEACHER', '0001_TW_ROLE_HOD'],
  },
  {
    id: 'staff-5',
    name: 'David Ng',
    cesEmail: 'david.ng@example.com',
    groups: ['0001_TWSTG_ROLE_TEACHER'],
  },
  {
    id: 'staff-6',
    name: 'Elena Foo',
    cesEmail: 'elena.foo@example.com',
    groups: ['0001_TW_ROLE_TEACHER', '1001_TW_ROLE_HOD'],
  },
  {
    id: 'staff-7',
    name: 'Jane Doe',
    cesEmail: 'jane.doe@example.com',
    groups: ['X_TW_ROLE_TEACHER', 'X_XX_ROLE_COUNSELLOR'],
  },
  {
    id: 'staff-8',
    name: 'Ivan Koh',
    groups: [],
  },
  {
    id: 'staff-9',
    name: 'Mary Koh',
    cesEmail: 'mk.only@example.com',
    groups: ['6101_TW_ROLE_TEACHER'],
  },
  {
    id: 'staff-10',
    name: 'Meng Keat',
    cesEmail: 'mk.mixed@example.com',
    groups: ['6101_TW_ROLE_HOD', '1234_TW_ROLE_TEACHER'],
  },
  {
    id: 'staff-11',
    name: 'Nora Teo',
    cesEmail: 'nora.teo@example.com',
    groups: ['1234_TW_ATTR_CCE'],
  },
];

/**
 * Returns the mock Edupass provider.
 *
 * @param config - Server URL and the TW client's registration.
 * @returns the {@link Provider}.
 */
export function createProvider(config: Config): Provider {
  const provider = new Provider(config.url, {
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
      openid: ['sub', 'iat', 'nbf', 'id', 'name', 'cesEmail', 'groups'],
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
      const account = accounts.find((a) => a.id === id);
      if (!account) {
        return undefined;
      }
      return {
        accountId: id,
        claims: async () => {
          const now = Math.floor(Date.now() / 1000);
          return {
            sub: account.id,
            iat: now,
            nbf: now,
            id: account.id,
            name: account.name,
            // cesEmail is the only claim here that Edupass omits when null.
            ...(account.cesEmail ? { cesEmail: account.cesEmail } : {}),
            groups: account.groups,
          };
        },
      };
    },
  });

  provider.proxy = true;

  return provider;
}
