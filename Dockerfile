# ---- build stage ----
FROM golang:1.26-bookworm AS build

# templ CLI for generating *_templ.go
RUN go install github.com/a-h/templ/cmd/templ@latest

# tailwind via bun (official image has it preinstalled)
COPY --from=oven/bun:1 /usr/local/bin/bun /usr/local/bin/bun

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN bun install && bun run build
RUN templ generate ./...
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /reno .

# ---- run stage ----
FROM alpine:3.21

# ca-certificates for outbound TLS (product URLs, gravatar-style lookups later);
# tzdata so autodate fields use a sane zone
RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /reno /app/reno
COPY --from=build /src/public /app/public

WORKDIR /app
VOLUME /app/pb_data

ENV RENO_ADDR="0.0.0.0:8090"

EXPOSE 8090

# PB exposes /api/health
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8090/api/health || exit 1

ENTRYPOINT ["/app/reno", "serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]
