# ssh-battleships

Battleships played over SSH: `ssh play.phons.dev`. Play the bot, or create a room and give a
friend the four-letter code. Your public key fingerprint is your account, so there is nothing to
sign up for and nothing to install.

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

## Deploying

One EC2 `t4g.nano` runs the container, which publishes the instance's port 22 to the server's
2222 (`compose.yaml`). Three things bite if skipped.

**The host key must survive redeploys.** `SSH_HOST_KEY` holds one base64 ed25519 private key and
the server writes it out at boot. Regenerate it and every returning player gets
`REMOTE HOST IDENTIFICATION HAS CHANGED`.

**The address must be an Elastic IP.** A stopped instance loses its automatic public IP, and this
one is stopped on purpose. The EIP is billed whether the instance runs or not.

**`play.phons.dev` must be DNS-only in Cloudflare.** The orange cloud proxies HTTP and HTTPS
only; SSH through it needs Spectrum.

`terraform/` builds all of it: the instance, its Elastic IP, the security group, an instance
profile for Session Manager, and the two schedules below. The secrets are deliberately not in
there, so put them in Parameter Store first and they never touch Terraform state:

```sh
aws ssm put-parameter --name /battleships/deploy-key --type SecureString \
  --value "$(cat ~/.ssh/battleships_deploy_key)"      # read-only github deploy key
aws ssm put-parameter --name /battleships/env --type SecureString --value "$(cat .env.production)"

cd terraform && terraform init && terraform apply
```

`/battleships/env` is the `.env` the container reads: `SSH_HOST_KEY`
(`base64 -w0 .ssh/battleships_ed25519`) plus the two Upstash variables. Point the
`play.phons.dev` A record at the `address` output, grey cloud.

Administration is Session Manager (`aws ssm start-session --target i-...`), so no admin SSH port
is ever open, and the instance's own sshd is disabled to leave port 22 to the game.

**Deploys happen at boot.** A oneshot unit pulls `main`, re-reads both parameters and runs
`docker compose up -d --build`, so every scheduled start ships whatever has landed. There is no
CI push, because the machine is off most of the week and most pushes would have nowhere to go. To
ship while it is up:

```sh
aws ssm send-command --instance-ids i-... \
  --document-name AWS-RunShellScript \
  --parameters 'commands=["/usr/local/bin/battleships-deploy"]'
```

## Opening hours

The server does not run all week. `restart: unless-stopped` plus an enabled docker service means
the game comes back on its own whenever the instance boots, so scheduling the box is the whole
mechanism.

```sh
aws ec2 start-instances --instance-ids i-...   # open now
aws ec2 stop-instances  --instance-ids i-...   # closed
```

The published hours are Friday to Sunday, 6pm to 11pm UK time: two EventBridge schedules against
the universal EC2 target, no Lambda involved, in `terraform/main.tf` as `open_cron` and
`close_cron`. The timezone is named rather than an offset, so the hours hold across BST and GMT.
They are also written out in one sentence in `www/src/pages/index.astro`, which nothing derives
from the cron, so change both.

A game still in progress at 11pm dies with the instance. Sessions are short and the audience is
one person at a time, so the fix (draining, or refusing to stop while a room is live) is not
worth building yet.

While it runs, the server writes a `battleships:live` key to Redis every minute with a 150 second
expiry, and deletes it on a clean shutdown. That key is the only thing the landing page's live
badge reads, so the badge follows reality whether the instance was stopped on schedule, stopped
by hand, or fell over.

Stopped hours cost nothing in compute. The floor is the Elastic IP and the 8GB volume, about
$4.30/mo, and evenings-only running adds well under a dollar.
