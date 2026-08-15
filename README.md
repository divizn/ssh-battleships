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

A single DigitalOcean droplet runs the container, which publishes the droplet's port 22 to the
server's 2222 (`compose.yaml`). Three things bite if skipped.

**The host key must survive redeploys.** `SSH_HOST_KEY` holds one base64 ed25519 private key and
the server writes it out at boot. Regenerate it and every returning player gets
`REMOTE HOST IDENTIFICATION HAS CHANGED`.

**The droplet's own sshd has to move off port 22 first**, or the container cannot bind it. Open
the new port and confirm you can log in on it *before* closing the old one.

**`play.phons.dev` must be DNS-only in Cloudflare.** The orange cloud proxies HTTP and HTTPS
only; SSH through it needs Spectrum.

On the droplet, once:

```sh
sed -i 's/^#\?Port 22$/Port 2200/' /etc/ssh/sshd_config && systemctl restart ssh
ufw allow 2200/tcp && ufw allow 22/tcp && ufw enable
curl -fsSL https://get.docker.com | sh

# 512MB is enough to run the game but not to compile it
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

Then, per deploy:

```sh
git pull
docker compose up -d --build
```

`.env` on the droplet holds `SSH_HOST_KEY` (`base64 -w0 .ssh/battleships_ed25519`) alongside the
two Upstash variables. Point the `play.phons.dev` A record at the droplet's IP, grey cloud.

Other projects share the box by publishing their own ports from their own compose files. A load
balancer only earns its $12/mo once there is more than one droplet to balance across; until then
it is a second thing to configure and no failover.
