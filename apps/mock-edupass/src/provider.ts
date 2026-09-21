import Provider from 'oidc-provider';

const validAccounts = [
  {
    sub: 'staff-1',
    email: 'john.smith@example.com',
    name: 'John Smith',
    groups: ['X_TW_ROLE_TEACHER', 'X_TW_ATTR_PG_ADMIN'],
  },
  {
    sub: 'staff-2',
    email: 'alice.tan@example.com',
    name: 'Alice Tan',
    groups: ['1234_TW_ROLE_TEACHER'],
  },
];

const invalidAccounts = [
  {
    sub: 'iv-staff-01',
    email: 'jane.doe@example.com',
    name: 'Jane Doe',
    groups: ['X_TW_ROLE_TEACHER', 'X_ROLE_COUNSELLOR'],
  },
  {
    sub: 'iv-staff-02',
    email: 'no-name@example.com',
    groups: [] as string[],
  },
];

export const accounts = [...validAccounts, ...invalidAccounts];

export function createProvider(port: number): Provider {
  const issuer = `http://localhost:${port}`;

  return new Provider(issuer, {
    clients: [
      {
        client_id: 'teacher-workspace',
        client_secret: 'teacher-workspace-secret',
        redirect_uris: ['http://localhost:3000/auth/edupass/callback'],
        response_types: ['code'],
        grant_types: ['authorization_code'],
        token_endpoint_auth_method: 'client_secret_post',
      },
    ],

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
          claims.groups = account.groups;
          return claims;
        },
      };
    },
  });
}
