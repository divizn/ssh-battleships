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
