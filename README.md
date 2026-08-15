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

One EC2 `t4g.nano` runs the container, which publishes the instance's port 22 to the server's
2222 (`compose.yaml`). Three things bite if skipped.

**The host key must survive redeploys.** `SSH_HOST_KEY` holds one base64 ed25519 private key and
the server writes it out at boot. Regenerate it and every returning player gets
`REMOTE HOST IDENTIFICATION HAS CHANGED`.

**The address must be an Elastic IP.** A stopped instance loses its automatic public IP, and this
one is stopped on purpose. The EIP is billed whether the instance runs or not.

**`play.phons.dev` must be DNS-only in Cloudflare.** The orange cloud proxies HTTP and HTTPS
only; SSH through it needs Spectrum.

Launch Amazon Linux 2023 on arm64 with an instance profile carrying
`AmazonSSMManagedInstanceCore`, a security group allowing inbound TCP 22 from `0.0.0.0/0` and
`::/0`, and an Elastic IP. Administration is Session Manager (`aws ssm start-session --target
i-...`), so no admin SSH port is ever open. Then, once:

```sh
systemctl disable --now sshd          # frees port 22 for the game
dnf install -y docker git && systemctl enable --now docker

# 512MB runs the game but will not compile it
dd if=/dev/zero of=/swapfile bs=1M count=2048 && chmod 600 /swapfile
mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

The repo is private, so give the instance a read-only GitHub deploy key. Then, per deploy:

```sh
git pull
docker compose up -d --build
```

`.env` on the instance holds `SSH_HOST_KEY` (`base64 -w0 .ssh/battleships_ed25519`) alongside the
two Upstash variables. Point the `play.phons.dev` A record at the Elastic IP, grey cloud.

## Opening hours

The server does not run all week. `restart: unless-stopped` plus an enabled docker service means
the game comes back on its own whenever the instance boots, so scheduling the box is the whole
mechanism.

```sh
aws ec2 start-instances --instance-ids i-...   # open now
aws ec2 stop-instances  --instance-ids i-...   # closed
```

The published hours are Friday to Sunday, 6pm to 11pm UK time, which is two EventBridge schedules
against the universal EC2 target, no Lambda involved:

```sh
aws scheduler create-schedule --name battleships-open \
  --schedule-expression "cron(0 18 ? * FRI-SUN *)" \
  --schedule-expression-timezone Europe/London \
  --flexible-time-window '{"Mode":"OFF"}' \
  --target '{"Arn":"arn:aws:scheduler:::aws-sdk:ec2:startInstances","RoleArn":"<scheduler-role>","Input":"{\"InstanceIds\":[\"i-...\"]}"}'

aws scheduler create-schedule --name battleships-closed \
  --schedule-expression "cron(0 23 ? * FRI-SUN *)" \
  --schedule-expression-timezone Europe/London \
  --flexible-time-window '{"Mode":"OFF"}' \
  --target '{"Arn":"arn:aws:scheduler:::aws-sdk:ec2:stopInstances","RoleArn":"<scheduler-role>","Input":"{\"InstanceIds\":[\"i-...\"]}"}'
```

The timezone is named rather than an offset, so the hours stay put across BST and GMT. The role
needs only `ec2:StartInstances` and `ec2:StopInstances`, trusted by `scheduler.amazonaws.com`.
The hours are also written out in one sentence on the landing page, which is a separate repo:
change them here and change them there.

A game still in progress at 11pm dies with the instance. Sessions are short and the audience is
one person at a time, so the fix (draining, or refusing to stop while a room is live) is not
worth building yet.

While it runs, the server writes a `battleships:live` key to Redis every minute with a 150 second
expiry, and deletes it on a clean shutdown. That key is the only thing the landing page's live
badge reads, so the badge follows reality whether the instance was stopped on schedule, stopped
by hand, or fell over.

Stopped hours cost nothing in compute. The floor is the Elastic IP and the 8GB volume, about
$4.30/mo, and evenings-only running adds well under a dollar.
