# Mock Edupass

A local OIDC provider that stands in for [Edupass](https://sites.google.com/moe.edu.sg/mims-v2),
MOE's identity provider. It exists so the sign-in surface can be built and tested on a developer
machine and in CI, with no Edupass onboarding, no real credentials, and no MOE network access.

It is built on [`oidc-provider`](https://github.com/panva/node-oidc-provider), hosted on Express,
and is for local development and CI only. It is never deployed.

## Running it

From the repository root:

```bash
pnpm dev:mock-edupass
```

It listens on <http://127.0.0.1:3002> and serves its discovery document at
<http://127.0.0.1:3002/.well-known/openid-configuration>. Set `PORT` to use a different port.
That is the only environment variable it reads.

## Signing in

There are no passwords. An authorization request lands on an account picker that lists every
account below, and picking one signs in as it.

| `sub`         | `email`                        | `name`          |
| ------------- | ------------------------------ | --------------- |
| `teacher-001` | `amirah.rahman@schools.gov.sg` | `Amirah Rahman` |
| `teacher-002` | `wei.ling.tan@schools.gov.sg`  | `Tan Wei Ling`  |
| `teacher-003` | `no.name@schools.gov.sg`       | none            |

`teacher-003` has no name on purpose. Edupass may omit claims, and a relying party that assumes
every claim is present should break against this account rather than in front of a teacher.

The accounts are hard-coded in [`src/accounts.ts`](src/accounts.ts). There is no database, no
config file, and no environment variable behind them.

## What it supports

- Authorization code flow with mandatory PKCE (`S256`)
- `response_mode=form_post`, so the authorization response is POSTed to the relying party
- A single confidential client, with the `openid` scope carrying `sub`, `email`, and `name`
  in the ID token
- Discovery and JWKS endpoints, both owned by the library

The registered client is `teacher-workspace`, with the redirect URI
`http://localhost:3000/auth/callback`, which is where the Go server's callback will live. The
client secret is in [`src/provider.ts`](src/provider.ts): it is a fake credential for a fake
provider and guards nothing.

## Testing

```bash
pnpm --filter @teacher-workspace/mock-edupass test
```

Tests use Node's built-in runner and drive the provider through a real HTTP client that performs
PKCE, follows the redirect chain, parses the `form_post` document, and verifies ID token
signatures against the published JWKS.

## Known gaps

Both are deliberate and tracked:

- It serves no `Content-Security-Policy` (#63). The `form_post` page relies on an inline script,
  and the policy has to be applied before the provider handler runs for the library to append the
  script's `sha256-` hash to `script-src`.
- It signs with the library's bundled development key, `kid: keystore-CHANGE-ME` (#64). That key
  is published in a public npm package, and its use is why startup logs a warning.
