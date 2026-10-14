# mock-edupass

A local OIDC provider that stands in for Edupass during development and CI testing. It implements the OpenID Connect Authorization Code flow with PKCE, auto-login (no UI), and hard-coded test accounts, so you can develop and test authentication without real credentials or MOE network access.

## Quick Start

```bash
# From the repository root
pnpm --filter @teacher-workspace/mock-edupass dev

pnpm --filter @teacher-workspace/mock-edupass start
```

`dev` fails when the repository's `.env` is missing. Copy `.env.example` to `.env` first.

- Server: `http://localhost:9000` (listens on `MOCK_EDUPASS_PORT`, served as `MOCK_EDUPASS_URL`)
- Discovery: `http://localhost:9000/.well-known/openid-configuration`
- Health check: `GET http://localhost:9000/health`

## Client Configuration

Configure your relying party with these values:

| Parameter | Value |
| --- | --- |
| Issuer | `MOCK_EDUPASS_URL`, `http://localhost:9000` in `.env.example` |
| Client ID | `MOCK_EDUPASS_TW_ID`, `teacher-workspace` in `.env.example` |
| Client secret | `MOCK_EDUPASS_TW_SECRET`, `teacher-workspace-secret` in `.env.example`, for `client_secret_post` |
| Certificate | `MOCK_EDUPASS_TW_CERTIFICATE`, the relying party's X.509 certificate, for `private_key_jwt` |
| Redirect URI | `MOCK_EDUPASS_TW_REDIRECT_URI`, `http://localhost:3000/auth/edupass/callback` in `.env.example` |
| Scopes | `openid` |
| Response type | `code` |
| Response mode | `query` |
| Grant type | `authorization_code` |
| Token endpoint auth | `MOCK_EDUPASS_TW_AUTH_METHOD`, `client_secret_post` or `private_key_jwt` |
| PKCE | Required (S256) |

## Fake Accounts

| Account ID (`sub`, `id`) | Name | CES email | Groups | Notes |
| --- | --- | --- | --- | --- |
| `staff-1` | John Smith | john.smith@example.com | `0001_TW_ROLE_TEACHER`, `0001_TW_ATTR_PG_ADMIN` | **Default.** Location-scoped role and attribute |
| `staff-2` | Alice Tan | alice.tan@example.com | `1234_TW_ROLE_TEACHER` | Single location-scoped role |
| `staff-3` | Bob Chen | bob.chen@example.com | `X_TW_ROLE_TEACHER`, `X_TW_ATTR_PG_ADMIN` | Global role (location=X) and attribute |
| `staff-4` | Carol Lim | carol.lim@example.com | `0001_TW_ROLE_TEACHER`, `0001_TW_ROLE_HOD` | **Conflict fixture:** two TW roles at same location |
| `staff-5` | David Ng | david.ng@example.com | `0001_TWSTG_ROLE_TEACHER` | Pre-prod TWSTG app code; refused unless Teacher Workspace sets `TW_EDUPASS_APP_CODE=TWSTG` |
| `staff-6` | Elena Foo | elena.foo@example.com | `0001_TW_ROLE_TEACHER`, `1001_TW_ROLE_HOD` | Different roles at different schools |
| `staff-7` | Jane Doe | jane.doe@example.com | `X_TW_ROLE_TEACHER`, `X_XX_ROLE_COUNSELLOR` | Non-TW role present; for unknown-role rejection testing |
| `staff-8` | Ivan Koh | _(absent)_ | `[]` | Empty groups and missing CES email |
| `staff-9` | Mary Koh | mk.only@example.com | `6101_TW_ROLE_TEACHER` | Only role is at an MOE Kindergarten (MK) school (location `6101`); for MK-filter rejection testing |
| `staff-10` | Meng Keat | mk.mixed@example.com | `6101_TW_ROLE_HOD`, `1234_TW_ROLE_TEACHER` | MK-anchored role alongside a role at a non-MK school; signs in as `ROLE_TEACHER` |
| `staff-11` | Grace Lee | grace.lee@example.com | `0001_TW_ROLE_PRINCIPAL` | Single non-teacher role; signs in as `ROLE_PRINCIPAL` |
| `staff-12` | Henry Wong | henry.wong@example.com | `1234_TW_ROLE_VICE_PRINCIPAL_ADMINISTRATION` | Single non-teacher role with the longest display name; signs in as `ROLE_VICE_PRINCIPAL_ADMINISTRATION` |
| `staff-13` | Nora Teo | nora.teo@example.com | `1234_TW_ATTR_CCE` | An attribute and no base role |

## How It Works

There is no login page or consent screen. Authentication and consent complete automatically:

- Defaults to `staff-1` unless `?account=<id>` is passed to the authorize endpoint
- Example: `/authorize?...&account=staff-2` logs in as Alice Tan

## Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `MOCK_EDUPASS_PORT` | `9000` | Port the server listens on |
| `MOCK_EDUPASS_URL` | None | Required. Public URL of the server, used as the OIDC issuer. Behind a load balancer, this is the public URL, not the container's port |
| `MOCK_EDUPASS_TW_ID` | None | Required. Client ID registered for the relying party; must match `TW_OIDC_CLIENT_ID` |
| `MOCK_EDUPASS_TW_REDIRECT_URI` | None | Required. Redirect URI registered for the relying party; must match `TW_OIDC_REDIRECT_URL` |
| `MOCK_EDUPASS_TW_AUTH_METHOD` | None | Required. `client_secret_post` or `private_key_jwt` |
| `MOCK_EDUPASS_TW_SECRET` | None | Client secret, required for `client_secret_post` |
| `MOCK_EDUPASS_TW_SECRET_FILE` | None | Path to a file that holds the client secret |
| `MOCK_EDUPASS_TW_CERTIFICATE` | None | Relying party's X.509 certificate, required for `private_key_jwt` |
| `MOCK_EDUPASS_TW_CERTIFICATE_FILE` | None | Path to a file that holds the certificate |

The server exits with an error at startup if you set both `MOCK_EDUPASS_TW_SECRET` and `MOCK_EDUPASS_TW_SECRET_FILE`, or both `MOCK_EDUPASS_TW_CERTIFICATE` and `MOCK_EDUPASS_TW_CERTIFICATE_FILE` ([`_FILE` variables](https://docs.docker.com/engine/swarm/secrets/#build-support-for-docker-secrets-into-your-images)).

## Client Authentication

`MOCK_EDUPASS_TW_AUTH_METHOD` sets how the relying party authenticates at the token endpoint ([OpenID Connect Core: Client Authentication](https://openid.net/specs/openid-connect-core-1_0.html#ClientAuthentication)):

- `client_secret_post`: the relying party sends `client_secret` in the token request.
- `private_key_jwt`: the relying party sends a `client_assertion` JWT signed with its private key, as [Edupass](https://docs.google.com/document/d/1Fb2tbkKOHy_pUxbipwmGj4RWkVVW1eT_/edit#heading=h.yek85phwajdc) requires. mock-edupass holds only the relying party's certificate, and accepts a client assertion only when it is signed with `PS256` and its `x5t#S256` header parameter matches the certificate thumbprint.

## Signing Keys

The provider generates its own RSA-2048 signing key on every boot and publishes the public half at `jwks_uri`. It is not configurable and is not persisted, so the `kid` changes on every restart and a relying party has to refetch the JWKS.

This replaces the development keystore bundled in the `oidc-provider` package, whose private half is public and which the library warns about at startup.

## Running Tests

```bash
pnpm --filter @teacher-workspace/mock-edupass test
```
