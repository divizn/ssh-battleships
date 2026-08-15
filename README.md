# ssh-battleships

Battleships played over SSH: `ssh play.phons.dev`. Play the bot, or create a room and give a
friend the four-letter code. Your public key fingerprint is your account, so there is nothing to
sign up for and nothing to install.

The landing page lives in a separate repo and is served at `battleships.phons.dev`.

## Running it

```sh
go run ./cmd/server -local   # play in this terminal, no ssh, no network
go run ./cmd/server          # serve ssh on :2222
go test ./...
```

Names, records and the leaderboard need `UPSTASH_REDIS_REST_URL` and `UPSTASH_REDIS_REST_TOKEN`,
read from `.env` (see `.env.example`) or the environment. Without them the game runs and simply
remembers nothing.

## Deploying

Fly runs the binary; the edge maps port 22 to 2222 (`fly.toml`). Two things bite if skipped.

**The host key must survive deploys.** `SSH_HOST_KEY` holds one base64 ed25519 private key and
the server writes it out at boot. Regenerate it and every returning player gets
`REMOTE HOST IDENTIFICATION HAS CHANGED`.

**`play.phons.dev` must be DNS-only in Cloudflare.** The orange cloud proxies HTTP and HTTPS
only; SSH through it needs Spectrum.

```sh
fly launch --no-deploy --copy-config
fly secrets set SSH_HOST_KEY="$(base64 -w0 .ssh/battleships_ed25519)"
fly secrets set UPSTASH_REDIS_REST_URL=... UPSTASH_REDIS_REST_TOKEN=...
fly deploy
fly ips list   # the A record play.phons.dev points at, grey cloud
```

The machine holds live games in memory, which is why `fly.toml` keeps one always running rather
than stopping it when idle.
