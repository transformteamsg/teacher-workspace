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

| Parameter           | Value                                 |
| ------------------- | ------------------------------------- |
| Issuer              | `http://localhost:9000`               |
| Client ID           | `teacher-workspace`                   |
| Client Secret       | `teacher-workspace-secret`            |
| Redirect URI        | `http://localhost:3000/auth/callback` |
| Scopes              | `openid`                              |
| Response type       | `code`                                |
| Response mode       | `form_post`                           |
| Grant type          | `authorization_code`                  |
| Token endpoint auth | `client_secret_post`                  |
| PKCE                | Required (S256)                       |

## Fake Accounts

| Account ID  | Email                  | Name       | Notes                                    |
| ----------- | ---------------------- | ---------- | ---------------------------------------- |
| `teacher-1` | jane.doe@example.com   | Jane Doe   | Default (used when no account specified) |
| `teacher-2` | john.smith@example.com | John Smith |                                          |
| `teacher-3` | no-name@example.com    | _(absent)_ | For testing missing name claim           |

## How It Works

There is no login page or consent screen. Authentication and consent complete automatically:

- Defaults to `teacher-1` unless `?account=<id>` is passed to the authorize endpoint
- Example: `/authorize?...&account=teacher-2` logs in as John Smith

## Environment Variables

| Variable            | Default | Description                |
| ------------------- | ------- | -------------------------- |
| `MOCK_EDUPASS_PORT` | `9000`  | Port the server listens on |

## Running Tests

```bash
pnpm --filter @teacher-workspace/mock-edupass test
```
