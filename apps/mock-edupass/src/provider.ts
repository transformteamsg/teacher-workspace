import Provider, { errors } from 'oidc-provider';

import { type ClientKey, generateSigningJwk } from './jwks.ts';

export const accounts = [
  { sub: 'teacher-1', email: 'jane.doe@example.com', name: 'Jane Doe' },
  { sub: 'teacher-2', email: 'john.smith@example.com', name: 'John Smith' },
  { sub: 'teacher-3', email: 'no-name@example.com' },
];

/**
 * Returns the mock Edupass provider, with private_key_jwt as the only client authentication.
 *
 * `oidc-provider` never reads `x5t#S256`, so the `assertJwtClientAuthClaimsAndHeader` hook is
 * what rejects a wrong thumbprint. Signing keys are generated per boot, never the bundled one.
 *
 * @param port - Port the provider is reached on, fixing the issuer at `http://localhost:<port>`.
 * @param clientKey - The backend client's JWK and certificate thumbprint.
 * @returns A provider carrying the backend as its only registered client.
 */
export function createProvider(port: number, clientKey: ClientKey): Provider {
  const issuer = `http://localhost:${port}`;

  return new Provider(issuer, {
    jwks: { keys: [generateSigningJwk()] },

    clients: [
      {
        client_id: 'teacher-workspace',
        redirect_uris: ['http://localhost:3000/auth/edupass/callback'],
        response_types: ['code'],
        grant_types: ['authorization_code'],
        token_endpoint_auth_method: 'private_key_jwt',
        token_endpoint_auth_signing_alg: 'PS256',
        jwks: { keys: [clientKey.jwk] },
      },
    ],

    assertJwtClientAuthClaimsAndHeader: async (_ctx, _claims, header) => {
      if (header['x5t#S256'] !== clientKey.certificateThumbprint) {
        throw new errors.InvalidClientAuth('x5t#S256 does not match the registered certificate');
      }
    },

    claims: {
      openid: ['sub', 'email', 'name'],
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
          const claims: { sub: string; [key: string]: string } = { sub: account.sub };
          if (account.email) claims.email = account.email;
          if (account.name) claims.name = account.name;
          return claims;
        },
      };
    },
  });
}
