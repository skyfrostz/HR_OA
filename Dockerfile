# syntax=docker/dockerfile:1
FROM golang:1.26.4-bookworm AS api-build
ARG OA_BUILD_VERSION=development
WORKDIR /src
COPY server/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    printf '%s\n' "$OA_BUILD_VERSION" | grep -Eq '^(development|[0-9a-f]{7,40}|v[0-9]+(\.[0-9]+){2})$' && \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags "-X main.buildVersion=$OA_BUILD_VERSION" -o /out/hr-oa ./cmd/oa

FROM alpine:3.23.4 AS api
RUN addgroup -S -g 10001 oa && adduser -S -D -H -u 10001 -G oa oa \
    && install -d -o oa -g oa -m 0700 /var/lib/oa \
    && test -s /etc/ssl/certs/ca-certificates.crt
COPY --from=api-build /out/hr-oa /usr/local/bin/hr-oa
USER 10001:10001
EXPOSE 8888
ENTRYPOINT ["/usr/local/bin/hr-oa"]

FROM node:24.15.0-bookworm-slim AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build:oa

FROM node:24.15.0-bookworm-slim AS pure-build
WORKDIR /src
COPY web-pure-admin/package.json web-pure-admin/pnpm-lock.yaml ./
# Keep the two required upstream settings without copying registry credentials.
RUN printf 'shamefully-hoist=true\nstrict-peer-dependencies=false\n' > .npmrc
RUN --mount=type=cache,target=/root/.npm --mount=type=cache,target=/root/.local/share/pnpm/store \
    npx --yes pnpm@10.32.1 install --frozen-lockfile --ignore-scripts
COPY web-pure-admin/ ./
RUN --mount=type=cache,target=/root/.npm npx --yes pnpm@10.32.1 run build

FROM nginx:1.28.0-alpine AS web
COPY deploy/oa/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-build /src/dist-oa/h5.html /usr/share/nginx/html/h5.html
COPY --from=web-build /src/dist-oa/assets/ /usr/share/nginx/html/assets/
COPY --from=pure-build /src/dist/oa.html /usr/share/nginx/html/oa.html
COPY --from=pure-build /src/dist/pure-assets/ /usr/share/nginx/html/pure-assets/
EXPOSE 8080
