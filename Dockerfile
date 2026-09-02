# syntax=docker/dockerfile:1

# ----------------------------------------
# TW Host build stage
# ----------------------------------------
FROM node:24.19.0-trixie AS tw-host-build

RUN mkdir /app
WORKDIR /app

ENV NODE_ENV=production

# Install `pnpm`.
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN mkdir $PNPM_HOME && \
        wget -qO- "https://github.com/pnpm/pnpm/releases/download/v11.22.0/pnpm-linux-arm64.tar.gz" | tar -xzf - -C "$PNPM_HOME" && \
        ln -s $PNPM_HOME/pnpm /usr/local/bin/pnpm

# Fetch all dependencies into the virtual store.
COPY pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm fetch

COPY package.json ./
COPY apps/host/package.json ./apps/host/package.json

RUN pnpm install --offline --frozen-lockfile --ignore-scripts

COPY apps/host/ ./apps/host/

# Build the host app.
RUN pnpm --filter=@teacher-workspace/host build

# ----------------------------------------
# TW Server build stage
# ----------------------------------------
FROM golang:1.26.5-trixie AS tw-server-build

RUN mkdir /app
WORKDIR /app

# Fetch all dependencies.
COPY go.mod go.sum ./
RUN go mod download

COPY server/ ./server

RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" ./server/cmd/tw

# ----------------------------------------
# Production stage
# ----------------------------------------
FROM debian:trixie-slim

RUN mkdir /app
WORKDIR /app

# 1. Create a system group named `zero`.
# 2. Create a system user named `zero` (no home directory, no login shell).
# 3. Change the permission of `app` folder to user `zero`.
RUN groupadd --system zero && \
        useradd --system --gid zero --no-create-home --shell /usr/sbin/nologin zero && \
        chown zero:zero /app

USER zero

COPY --from=tw-host-build --chown=zero:zero /app/apps/host/dist /app/dist
COPY --from=tw-server-build --chown=zero:zero /app/tw /app/tw

ENV TW_ENV=production \
    TW_BUILD_DIR=/app/dist \
    TW_SERVER_PORT=3000

EXPOSE 3000

CMD ["/app/tw"]
