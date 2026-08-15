# battleships-site

Landing page for [ssh-battleships](https://github.com/divizn/ssh-battleships), the terminal game
at `ssh play.phons.dev`. Astro on Cloudflare Workers, served at `battleships.phons.dev`.

The page is rendered per request so the leaderboard is live, with a 60 second cache in front of
it. It reads the same Upstash database the game writes to, under the `battleships:` namespace,
and renders without a leaderboard if Redis is missing or unreachable.

The game server does not run all week. It writes `battleships:live` every minute with a 150
second expiry, which is what the badge at the top of the page reads, so the badge can lag reality
by up to a minute of page cache. The scheduled hours are one sentence in `index.astro`; the
schedule itself lives in EventBridge, in the game's own repo.

```sh
pnpm install
pnpm dev       # needs .dev.vars, below
pnpm preview   # build, then wrangler dev
pnpm deploy    # build, then wrangler deploy
```

Locally the credentials go in `.dev.vars` (gitignored):

```
UPSTASH_REDIS_REST_URL=...
UPSTASH_REDIS_REST_TOKEN=...
```

In production the same two are Worker secrets:

```sh
wrangler secret put UPSTASH_REDIS_REST_URL
wrangler secret put UPSTASH_REDIS_REST_TOKEN
```

`battleships.phons.dev` is a normal proxied Cloudflare record. `play.phons.dev`, which points at
the droplet running the game, must stay DNS-only: the proxy does not carry SSH.
