# ---- frontend build ----
FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- go build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf embedded/frontend
COPY --from=frontend /src/web/out ./embedded/frontend
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin ./cmd/plex2jellyfin && \
    go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin-daemon ./cmd/plex2jellyfin-daemon && \
    go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin-web ./cmd/plex2jellyfin-web

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache tini su-exec
COPY --from=build /out/ /usr/local/bin/
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENV PUID=1000 PGID=1000 HOME=/config
VOLUME ["/config", "/watch", "/library"]
EXPOSE 5522
ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
