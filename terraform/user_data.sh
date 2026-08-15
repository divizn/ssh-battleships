#!/bin/bash
set -euxo pipefail

dnf install -y docker git

install -m 0755 -d /usr/libexec/docker/cli-plugins
curl -fsSL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 \
  -o /usr/libexec/docker/cli-plugins/docker-compose
chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# the al2023 docker package ships buildx 0.12.1, older than the compose above will build with
tag=$(curl -fsSL https://api.github.com/repos/docker/buildx/releases/latest | grep -m1 tag_name | cut -d: -f2 | tr -d ' ",')
curl -fsSL https://github.com/docker/buildx/releases/download/$tag/buildx-$tag.linux-arm64 \
  -o /usr/libexec/docker/cli-plugins/docker-buildx
chmod +x /usr/libexec/docker/cli-plugins/docker-buildx

systemctl enable --now docker

# session manager is the way in, so nothing here needs sshd and the game can have port 22
# masked rather than disabled, because cloud-init starts sshd again on every boot
systemctl mask --now sshd.service sshd.socket

# 512MB runs the game but will not compile it
dd if=/dev/zero of=/swapfile bs=1M count=2048
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

cat > /usr/local/bin/${name}-deploy <<'DEPLOY'
#!/bin/bash
set -euo pipefail

app=/srv/${name}

if [ -d $app/.git ]; then
  git -C $app fetch --prune origin
  git -C $app reset --hard origin/main
else
  git clone ${repo} $app
fi

aws ssm get-parameter --name /${name}/env --with-decryption \
  --query Parameter.Value --output text > $app/.env
chmod 600 $app/.env

docker compose -f $app/compose.yaml up -d --build
DEPLOY
chmod +x /usr/local/bin/${name}-deploy

# every boot ships whatever is on main, which is the whole deploy: the box is off most of the
# week, so there is no CI run that could usefully push to it
cat > /etc/systemd/system/${name}-deploy.service <<UNIT
[Unit]
Description=Pull and start battleships
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=AWS_DEFAULT_REGION=${region}
ExecStart=/usr/local/bin/${name}-deploy

[Install]
WantedBy=multi-user.target
UNIT

systemctl enable --now ${name}-deploy.service
