# ssh-battleships

Battleships played over SSH: `ssh play.phons.dev`. Play the bot, or create a room and give a
friend the four-letter code. Your public key fingerprint is your account, so there is nothing to
sign up for and nothing to install.

![a full game played in the terminal](https://cdn.phons.dev/battleships.gif)

```
cmd/ internal/   the game
www/             the landing page at battleships.phons.dev, Astro on Cloudflare Workers
terraform/       the AWS it runs on
```

The two halves deploy to different places and share only the Upstash database, so `www` is in
`.dockerignore` and never reaches the game's image.

## Running it

```sh
go run ./cmd/server -local   # play in this terminal, no ssh, no network
go run ./cmd/server          # serve ssh on :2222
go test ./...
```

Names, records and the leaderboard need `UPSTASH_REDIS_REST_URL` and `UPSTASH_REDIS_REST_TOKEN`,
read from `.env` (see `.env.example`) or the environment. Without them the game runs and simply
remembers nothing.

## Opening hours

The server is up Friday to Sunday, 6pm to 11pm UK time, and the machine is stopped the rest of
the week. `restart: unless-stopped` plus an enabled docker service means the game comes back on
its own whenever the machine boots, so scheduling the machine is the whole mechanism; the
schedule is `open_cron` and `close_cron` in `terraform/main.tf`. The hours are written out again
in one sentence in `www/src/pages/index.astro`, which nothing derives from the cron, so change
both.

A game still in progress at 11pm dies with the machine. Sessions are short and the audience is
one person at a time, so the fix (draining, or refusing to stop while a room is live) is not
worth building yet.

While it runs, the server writes a `battleships:live` key to Redis every minute with a 150 second
expiry, and deletes it on a clean shutdown. That key is the only thing the landing page's live
badge reads, so the badge follows reality whether the machine was stopped on schedule, stopped by
hand, or fell over.
