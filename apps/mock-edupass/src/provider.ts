import Provider from 'oidc-provider';

export const accounts = [
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
    groups: ['X_TW_ROLE_TEACHER', 'X_ROLE_COUNSELLOR'],
  },
  {
    sub: 'staff-8',
    email: 'no-name@example.com',
    groups: [] as string[],
  },
];

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
