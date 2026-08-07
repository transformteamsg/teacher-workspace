import Provider from 'oidc-provider';

import { accounts } from './accounts.ts';
import { config } from './config.ts';

const issuer = `http://localhost:${config.port}`;

const provider = new Provider(issuer, {
  clients: [
    {
      client_id: 'teacher-workspace',
      client_secret: 'teacher-workspace-secret',
      redirect_uris: ['http://localhost:3000/auth/callback'],
      response_types: ['code'],
      grant_types: ['authorization_code'],
      token_endpoint_auth_method: 'client_secret_post',
    },
  ],

  claims: {
    openid: ['sub', 'email', 'name'],
  },

  extraParams: ['account'],

  pkce: {
    required: () => true,
  },

  features: {
    devInteractions: { enabled: false },
  },

  cookies: {
    keys: ['fake-edupass-cookie-key'],
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
        const claims: Record<string, string> = { sub: account.sub };
        if (account.email) claims.email = account.email;
        if (account.name) claims.name = account.name;
        return claims;
      },
    };
  },
});

export { provider };
