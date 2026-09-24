# mock-edupass

A local OIDC provider that stands in for Edupass during development and CI testing. It implements the OpenID Connect Authorization Code flow with PKCE, auto-login (no UI), and hard-coded test accounts, so you can develop and test authentication without real credentials or MOE network access.

## Quick Start

```bash
pnpm --filter @teacher-workspace/mock-edupass dev
```

- Server: `http://localhost:9000` (configurable via `MOCK_EDUPASS_PORT`)
- Discovery: `http://localhost:9000/.well-known/openid-configuration`
- Health check: `GET http://localhost:9000/health`

Run `cp .env.example .env` at the repository root first. Both scripts load that file with `--env-file-if-exists=../../.env`, and `MOCK_EDUPASS_TW_PUBLIC_KEY` has no default anywhere in this package.

`dev` mints a key pair on its first run, into `.certs/` at the repository root, and reuses it on every run after that:

- `public.cer`, the self-signed certificate the provider registers for the backend client
- `private.key`, the private half the backend signs its `client_assertion` with, written `0600` inside a `0700` directory

Neither is committed: `.certs/`, `*.key`, and `*.cer` are all ignored. `dev` generates the pair but never points the variable at it, so the value always comes from the environment. `.env.example` sets `MOCK_EDUPASS_TW_PUBLIC_KEY=../../.certs/public.cer`, relative to `apps/mock-edupass` because that is this package's working directory, while the backend's `TW_OIDC_CLIENT_*` paths in the same file are relative to the repository root. Both name the same two files. An exported variable beats the `.env` entry:

```bash
MOCK_EDUPASS_TW_PUBLIC_KEY=./public.cer \
  pnpm --filter @teacher-workspace/mock-edupass dev
```

Delete `.certs/` to mint a fresh pair.

`start` is the deployed path and generates nothing. It runs `src/index.ts` alone, which has no fallback, so `MOCK_EDUPASS_TW_PUBLIC_KEY` is required. The `-if-exists` form of the flag is what lets it boot in a deployment that ships no `.env`:

```bash
MOCK_EDUPASS_TW_PUBLIC_KEY="$(cat public.cer)" \
  pnpm --filter @teacher-workspace/mock-edupass start
```

With the variable unset, `start` exits non-zero before listening, and it does the same for a value that is unreadable, not RSA, or holds a private key. That is what a deployment wants, where the certificate comes from a secret store rather than the filesystem.

## Client Configuration

Configure your relying party with these values:

| Parameter           | Value                                         |
| ------------------- | --------------------------------------------- |
| Issuer              | `http://localhost:9000`                       |
| Client ID           | `teacher-workspace`                           |
| Redirect URI        | `http://localhost:3000/auth/edupass/callback` |
| Scopes              | `openid`                                      |
| Response type       | `code`                                        |
| Grant type          | `authorization_code`                          |
| Token endpoint auth | `private_key_jwt`                             |
| Assertion algorithm | `PS256`                                       |
| PKCE                | Required (S256)                               |

Each token request carries a `client_assertion`: a short-lived JWT signed with the private half of the key pair above. When `MOCK_EDUPASS_TW_PUBLIC_KEY` holds a certificate, the assertion must also carry an `x5t#S256` header equal to the base64url SHA-256 of that certificate's DER encoding, which is what Edupass expects. Anything that does not verify is refused with `invalid_client` (HTTP 401).

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

| Variable | Default | Description |
| --- | --- | --- |
| `MOCK_EDUPASS_PORT` | `9000` | Port the server listens on |
| `MOCK_EDUPASS_TW_PUBLIC_KEY` | _(none)_ | The backend client's X.509 certificate. Always required. `.env.example` points it at the certificate `dev` generates |

`MOCK_EDUPASS_TW_PUBLIC_KEY` accepts two forms:

- a PEM X.509 certificate (`-----BEGIN CERTIFICATE-----`)
- a path to one

A value starting with `-----BEGIN` is treated as the material itself, anything else as a path. A deployment with no writable disk can therefore carry the whole certificate in the variable. A private key is refused rather than reduced to its public half.

A bare public key is refused too. `x5t#S256` is the SHA-256 of the certificate's DER encoding, so a key on its own leaves nothing to compare the header against, and the check would pass silently for any value. Edupass registers the client by certificate for the same reason, and the backend's `TW_OIDC_CLIENT_PUBLIC_KEY` has always required one.

## Signing Keys

The provider generates its own RSA-2048 signing key on every boot and publishes the public half at `jwks_uri`. It is not configurable and is not persisted, so the `kid` changes on every restart and a relying party has to refetch the JWKS.

This replaces the development keystore bundled in the `oidc-provider` package, whose private half is public and which the library warns about at startup.

## Running Tests

```bash
pnpm --filter @teacher-workspace/mock-edupass test
```
