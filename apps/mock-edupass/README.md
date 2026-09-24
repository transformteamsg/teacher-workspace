# mock-edupass

A local OIDC provider that stands in for Edupass during development and CI testing. It implements the OpenID Connect Authorization Code flow with PKCE, auto-login (no UI), and hard-coded test accounts, so you can develop and test authentication without real credentials or MOE network access.

## Quick Start

```bash
# From the repository root
pnpm --filter @teacher-workspace/mock-edupass start

# Or from the app directory
cd apps/mock-edupass && pnpm start
```

- Server: `http://localhost:9000` (configurable via `MOCK_EDUPASS_PORT`)
- Discovery: `http://localhost:9000/.well-known/openid-configuration`
- Health check: `GET http://localhost:9000/health`

## Client Configuration

Configure your relying party with these values:

| Parameter           | Value                                         |
| ------------------- | --------------------------------------------- |
| Issuer              | `http://localhost:9000`                       |
| Client ID           | `teacher-workspace`                           |
| Client Secret       | `teacher-workspace-secret`                    |
| Redirect URI        | `http://localhost:3000/auth/edupass/callback` |
| Scopes              | `openid`                                      |
| Response type       | `code`                                        |
| Response mode       | `form_post`                                   |
| Grant type          | `authorization_code`                          |
| Token endpoint auth | `client_secret_post`                          |
| PKCE                | Required (S256)                               |

## Fake Accounts

| Account ID | Email                  | Name       | Groups                                          | Notes                                                   |
| ---------- | ---------------------- | ---------- | ----------------------------------------------- | ------------------------------------------------------- |
| `staff-1`  | john.smith@example.com | John Smith | `0001_TW_ROLE_TEACHER`, `0001_TW_ATTR_PG_ADMIN` | **Default.** Location-scoped role and attribute         |
| `staff-2`  | alice.tan@example.com  | Alice Tan  | `1234_TW_ROLE_TEACHER`                          | Single location-scoped role                             |
| `staff-3`  | bob.chen@example.com   | Bob Chen   | `X_TW_ROLE_TEACHER`, `X_TW_ATTR_PG_ADMIN`       | Global role (location=X) and attribute                  |
| `staff-4`  | carol.lim@example.com  | Carol Lim  | `0001_TW_ROLE_TEACHER`, `0001_TW_ROLE_HOD`      | **Conflict fixture:** two TW roles at same location     |
| `staff-5`  | david.ng@example.com   | David Ng   | `0001_TWSTG_ROLE_TEACHER`                       | Pre-prod TWSTG app code                                 |
| `staff-6`  | elena.foo@example.com  | Elena Foo  | `0001_TW_ROLE_TEACHER`, `1001_TW_ROLE_HOD`      | Different roles at different schools                    |
| `staff-7`  | jane.doe@example.com   | Jane Doe   | `X_TW_ROLE_TEACHER`, `X_ROLE_COUNSELLOR`        | Non-TW role present; for unknown-role rejection testing |
| `staff-8`  | no-name@example.com    | _(absent)_ | `[]`                                            | Empty groups and missing name                           |

## How It Works

There is no login page or consent screen. Authentication and consent complete automatically:

- Defaults to `staff-1` unless `?account=<id>` is passed to the authorize endpoint
- Example: `/authorize?...&account=staff-2` logs in as Alice Tan

## Environment Variables

| Variable            | Default | Description                |
| ------------------- | ------- | -------------------------- |
| `MOCK_EDUPASS_PORT` | `9000`  | Port the server listens on |

## Running Tests

```bash
pnpm --filter @teacher-workspace/mock-edupass test
```
