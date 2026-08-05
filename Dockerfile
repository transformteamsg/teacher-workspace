# syntax=docker/dockerfile:1

# ----------------------------------------
# Host build stage
# ----------------------------------------
FROM node:24-alpine3.23 AS host-build

# Keep in sync with the pnpm/action-setup version in .github/workflows/ci.yml,
# so the image and CI resolve dependencies identically.
ARG PNPM_VERSION=11.11.0

WORKDIR /src

RUN npm install --global pnpm@${PNPM_VERSION}

# Fetch all dependencies for better layer caching. Every workspace package
# matched by pnpm-workspace.yaml needs its manifest copied here, otherwise the
# frozen lockfile install fails on the missing importer.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/host/package.json apps/host/

# --ignore-scripts skips the root prepare script, which installs the lefthook
# git hooks and fails without a .git directory.
RUN pnpm install --frozen-lockfile --ignore-scripts

# Only apps/, so an edit under server/ leaves this stage cached.
COPY apps/ apps/

# Build the frontend into apps/host/dist.
RUN pnpm build

# ----------------------------------------
# Server build stage
# ----------------------------------------
FROM golang:1.26.5-alpine3.23 AS server-build

# BuildKit populates TARGETARCH from the platform being built. Hardcoding it
# would emit a binary for the wrong architecture whenever the build host is not
# arm64, which fails at run time rather than at build time.
ARG TARGETARCH

WORKDIR /src

# Fetch all dependencies for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY server/ server/

# Build the binary. CGO_ENABLED=0 keeps it static so it does not depend on the
# runtime stage's libc.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /src/tw ./server/cmd/tw

# ----------------------------------------
# Production stage
# ----------------------------------------
FROM alpine:3.23

WORKDIR /app

# No apk install step: alpine already ships ca-certificates-bundle, which is the
# trusted root store a static Go binary reads, and adduser/addgroup are busybox
# builtins. Keeping the stage offline makes the build reproducible.

# 1. Create a new user named `tw`.
# 2. Change the permission of `app` folder to user `tw`.
# 3. Change the current user from `root` to `tw`.
RUN addgroup -S tw && \
        adduser -S tw -G tw && \
        chown tw:tw /app

USER tw

COPY --from=server-build --chown=tw:tw /src/tw /app/tw
COPY --from=host-build --chown=tw:tw /src/apps/host/dist /app/dist

# The image ships no .env, so every setting comes from the environment.
# TW_BUILD_DIR is absolute because config.Validate resolves it against the
# working directory.
ENV TW_ENV=production \
    TW_BUILD_DIR=/app/dist \
    TW_SERVER_PORT=3000

EXPOSE 3000

# Exec form, so the server is PID 1 and receives SIGTERM for graceful shutdown.
CMD ["/app/tw"]
