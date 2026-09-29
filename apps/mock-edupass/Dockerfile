# syntax=docker/dockerfile:1

# Build from the repository root, which holds the pnpm lockfile and workspace:
#
#   docker build -f apps/mock-edupass/Dockerfile .

# ----------------------------------------
# Dependencies stage
# ----------------------------------------
FROM node:24.19.0-trixie AS deps

RUN mkdir /app
WORKDIR /app

# Install `pnpm`.
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN mkdir $PNPM_HOME && \
        wget -qO- "https://github.com/pnpm/pnpm/releases/download/v11.22.0/pnpm-linux-arm64.tar.gz" | tar -xzf - -C "$PNPM_HOME" && \
        ln -s $PNPM_HOME/pnpm /usr/local/bin/pnpm

# Fetch all dependencies into the store. Fetching also fills node_modules with
# every workspace package, so drop it and let the install below link only
# what mock-edupass needs.
COPY pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm fetch --prod && rm -rf node_modules

COPY package.json ./
COPY apps/mock-edupass/package.json ./apps/mock-edupass/package.json

# Link only mock-edupass's production dependencies.
RUN pnpm --filter=@teacher-workspace/mock-edupass install \
        --offline --frozen-lockfile --ignore-scripts --prod

# ----------------------------------------
# Production stage
# ----------------------------------------
FROM node:24.19.0-trixie-slim

ENV MOCK_EDUPASS_PORT=9000

WORKDIR /app

# Node runs the TypeScript sources directly, so there is no build step. The
# app's node_modules holds symlinks into the root virtual store, so both move.
COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/apps/mock-edupass/node_modules ./apps/mock-edupass/node_modules
COPY apps/mock-edupass/package.json ./apps/mock-edupass/package.json
COPY apps/mock-edupass/src ./apps/mock-edupass/src

WORKDIR /app/apps/mock-edupass

USER node

EXPOSE 9000

CMD ["node", "src/index.ts"]
