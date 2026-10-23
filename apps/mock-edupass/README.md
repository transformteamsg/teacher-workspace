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

| Account ID (`sub`, `id`) | Name | CES email | Groups | Notes | Result in Teacher Workspace |
| --- | --- | --- | --- | --- | --- |
| `staff-1` | John Smith | john.smith@example.com | `0001_TW_ROLE_TEACHER`, `0001_TW_ATTR_PG_ADMIN` | **Default.** Location-scoped role and attribute | Signs in as `ROLE_TEACHER` |
| `staff-2` | Alice Tan | alice.tan@example.com | `1234_TW_ROLE_TEACHER` | Single location-scoped role | Signs in as `ROLE_TEACHER` |
| `staff-3` | Bob Chen | bob.chen@example.com | `X_TW_ROLE_TEACHER`, `X_TW_ATTR_PG_ADMIN` | Global role (location=X) and attribute; for non-four-digit location rejection testing | Refused: the base role's location is not four digits, so no base role resolves |
| `staff-4` | Carol Lim | carol.lim@example.com | `0001_TW_ROLE_TEACHER`, `0001_TW_ROLE_HOD` | **Conflict fixture:** two TW roles at same location | Refused: two base roles resolve, and exactly one is required |
| `staff-5` | David Ng | david.ng@example.com | `0001_TWSTG_ROLE_TEACHER` | Pre-prod TWSTG app code | Refused: the app code is not `TW`, unless Teacher Workspace sets `TW_EDUPASS_APP_CODE=TWSTG` |
| `staff-6` | Elena Foo | elena.foo@example.com | `0001_TW_ROLE_TEACHER`, `1001_TW_ROLE_HOD` | Different roles at different schools | Refused: two base roles resolve, and exactly one is required |
| `staff-7` | Jane Doe | jane.doe@example.com | `0001_TW_ROLE_TEACHER`, `0001_XX_ROLE_COUNSELLOR` | Non-TW role alongside a valid TW role; for unknown-role rejection testing | Signs in as `ROLE_TEACHER`: the `XX` role is discarded |
| `staff-8` | Ivan Koh | _(absent)_ | `[]` | Empty groups and missing CES email | Refused: no CES email (the empty groups would also be refused) |
| `staff-9` | Mary Koh | mk.only@example.com | `6101_TW_ROLE_TEACHER` | Only role is at an MOE Kindergarten (MK) school (location `6101`); for MK-filter rejection testing | Refused: the MK role is discarded, so no base role resolves |
| `staff-10` | Meng Keat | mk.mixed@example.com | `6101_TW_ROLE_HOD`, `1234_TW_ROLE_TEACHER` | MK-anchored role alongside a role at a non-MK school | Signs in as `ROLE_TEACHER`: the MK role is discarded |
| `staff-11` | Grace Lee | grace.lee@example.com | `0001_TW_ROLE_PRINCIPAL` | Single non-teacher role | Signs in as `ROLE_PRINCIPAL` |
| `staff-12` | Henry Wong | henry.wong@example.com | `1234_TW_ROLE_VICE_PRINCIPAL_ADMINISTRATION` | Single non-teacher role with the longest display name | Signs in as `ROLE_VICE_PRINCIPAL_ADMINISTRATION` |
| `staff-13` | Nora Teo | nora.teo@example.com | `1234_TW_ATTR_CCE` | An attribute and no base role | Refused: no base role resolves |
| `staff-14` | Oscar Lee | oscar.lee@example.com | `1234_TW_ROLE_TEACHER`, `1234_TW_ROLE_HOD`, `5678_TW_ROLE_TEACHER` | **Conflict fixture:** two roles at one school and a role at another; for `multiple_roles` testing where both conflicts apply | Refused: three base roles resolve, and exactly one is required |
| `staff-15` | Priya Nair | priya.nair@example.com | `6101_TW_ROLE_TEACHER`, `1234_TW_ROLE_TEACHER`, `1234_TW_ROLE_HOD` | **Conflict fixture:** a role at an MK school does not count, but two roles at a non-MK school still give `multiple_roles` | Refused: the MK role is discarded, but two base roles still resolve, and exactly one is required |
| `staff-16` | Quentin Ong | quentin.ong@example.com | `6101_XX_ROLE_TEACHER` | Another application's role at an MK school | Refused with `no_role`, not `unsupported_school_mk`: the `XX` role is discarded, so no base role resolves |
| `staff-17` | Rachel Sim | rachel.sim@example.com | `6101_TW_ROLE_SUPERINTENDENT` | Unrecognized role code at an MK school | Refused with `no_role`, not `unsupported_school_mk`: the role code is unrecognized, so no base role resolves |
| `staff-18` | Samuel Yeo | samuel.yeo@example.com | `6101_TW_ATTR_CCE` | An attribute at an MK school and no base role | Refused with `no_role`: no base role resolves, and the attribute is kept |

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
