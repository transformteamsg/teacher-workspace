import Provider from 'oidc-provider';
import type { Configuration } from 'oidc-provider';

import { findAccount } from './accounts.ts';

/** CLIENT_ID identifies the teacher-workspace relying party, the only registered client. */
export const CLIENT_ID = 'teacher-workspace';

/**
 * CLIENT_SECRET is a fake credential for a provider that only ever runs on a developer
 * machine or in CI. It guards nothing, so it lives in source rather than in the environment.
 */
export const CLIENT_SECRET = 'teacher-workspace-secret';

/** REDIRECT_URI is the Go server's callback, which serves on port 3000 in development. */
export const REDIRECT_URI = 'http://localhost:3000/auth/callback';

const configuration: Configuration = {
  clients: [
    {
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET,
      redirect_uris: [REDIRECT_URI],
      response_types: ['code'],
      grant_types: ['authorization_code'],
      token_endpoint_auth_method: 'client_secret_basic',
    },
  ],

  // Edupass packs profile claims into the ID token under the openid scope alone, so a
  // relying party never has to ask for `profile` or `email`.
  claims: { openid: ['sub', 'email', 'name'] },
  scopes: ['openid'],

  // Defaults to true, which routes scope-requested claims to the UserInfo endpoint and
  // leaves the ID token carrying only `sub`.
  conformIdTokenClaims: false,

  // The bundled views accept any username and any password, and would shadow the account
  // picker this provider serves instead.
  features: { devInteractions: { enabled: false } },

  findAccount,

  // The library default requires PKCE only for public clients. This client is confidential,
  // so without the override an authorization request with no code_challenge would succeed.
  pkce: { required: () => true },
};

/**
 * createProvider builds a provider for the given issuer. The issuer must match the address
 * the server is reachable on, because discovery advertises absolute URLs built from it.
 */
export function createProvider(issuer: string): Provider {
  return new Provider(issuer, configuration);
}
