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

# Fetch all dependencies into the local store for better layer caching. This
# reads only pnpm-lock.yaml, so it doesn't need every workspace package's
# manifest copied in ahead of time.
COPY pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm fetch

# Only package.json and apps/, so an edit under server/ leaves this stage
# cached.
COPY package.json ./
COPY apps/ apps/

# --offline links from the store fetched above instead of hitting the network.
# --ignore-scripts skips the root prepare script, which installs the lefthook
# git hooks and fails without a .git directory.
RUN pnpm install --offline --frozen-lockfile --ignore-scripts

# Build the frontend into apps/host/dist.
RUN pnpm build

# ----------------------------------------
# Server build stage
# ----------------------------------------
FROM golang:1.26.5-alpine3.23 AS server-build

WORKDIR /src

# Fetch all dependencies for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

COPY server/ server/

# Build the binary. CGO_ENABLED=0 keeps it static so it does not depend on the
# runtime stage's libc.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /src/tw ./server/cmd/tw

# ----------------------------------------
# Production stage
# ----------------------------------------
FROM alpine:3.23

WORKDIR /app

# 1. Create a new user named `zero`.
# 2. Change the permission of `app` folder to user `zero`.
# 3. Change the current user from `root` to `zero`.
RUN addgroup -S zero && \
        adduser -S zero -G zero && \
        chown zero:zero /app

USER zero

COPY --from=server-build --chown=zero:zero /src/tw /app/tw
COPY --from=host-build --chown=zero:zero /src/apps/host/dist /app/dist

ENV TW_ENV=production \
    TW_BUILD_DIR=/app/dist \
    TW_SERVER_PORT=3000

EXPOSE 3000

CMD ["/app/tw"]
