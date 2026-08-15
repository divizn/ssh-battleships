# www

Landing page for the game in the rest of this repo, served at `battleships.phons.dev`. Astro on
Cloudflare Workers; nothing here ships in the game's container, which ignores this directory.

The page is rendered per request so the leaderboard is live, with a 60 second cache in front of
it. It reads the same Upstash database the game writes to, under the `battleships:` namespace,
and renders without a leaderboard if Redis is missing or unreachable.

The game server does not run all week. It writes `battleships:live` every minute with a 150
second expiry, which is what the badge at the top of the page reads, so the badge can lag reality
by up to a minute of page cache. The hours are one sentence in `index.astro`; the schedule that
actually turns the machine on and off is in `../terraform`, and the two have to be changed
together.

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

Every command here runs from this directory. Wired to Workers Builds instead, the root directory
is `www` and the build command is `pnpm build`.

`battleships.phons.dev` is a normal proxied Cloudflare record. `play.phons.dev`, which points at
the EC2 instance running the game, must stay DNS-only: the proxy does not carry SSH.
