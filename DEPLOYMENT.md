# Deployment

Reno ships as a single static binary serving both the app and the embedded
PocketBase (API + admin UI + SQLite + file storage). No external services
required.

## 1. Build

```sh
bun install && bun run build   # Tailwind CSS → public/app.css
templ generate ./...           # regenerate .templ → .go (build tools need this)
go build -o reno .
```

For other architectures cross-compile, e.g.:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reno-linux .
```

## 2. Run

```sh
./reno serve --http=127.0.0.1:8090 --dir=/var/lib/reno
```

- `--dir` holds `data.db` + uploaded files. Back it up; it IS the app state.
- First run prints a one-time superuser setup URL for the PocketBase admin UI
  at `/_/` (or use `./reno superuser upsert EMAIL PASS`).
- Create your user account through the app's `/register` page.
- Optional demo data: `./reno seed --dir=/var/lib/reno demo@reno.local demo1234`

## 3. Reverse proxy (TLS)

Point Caddy/nginx at the port; keep `/api/`, `/_/`, and `/files/` on the same
origin. Minimal Caddyfile:

```
reno.example.com {
    reverse_proxy 127.0.0.1:8090
}
```

Hardening checklist:

- Serve over HTTPS only (session cookie is `HttpOnly` + `SameSite=Lax`;
  add `Secure` in `internal/session/session.go` once you're on TLS).
- Set the PocketBase `--encryption-env` secret in production:
  `RENO_SECRET=$(openssl rand -hex 16) ./reno serve --encryption-env RENO_SECRET ...`
- Restrict admin UI (`/_/`) by IP at the proxy if possible.
- Back up `--dir` (cron + sqlite `.backup` or simple file copy while stopped).

## 4. systemd unit

```ini
[Unit]
Description=Reno
After=network.target

[Service]
User=reno
ExecStart=/opt/reno/reno serve --http=127.0.0.1:8090 --dir=/var/lib/reno
Environment=RENO_SECRET=change-me-32-bytes-of-random!!
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Upgrades

Replace the binary and restart. PocketBase runs schema migrations marked in
`migrations/` automatically on boot. Data dir stays untouched.
