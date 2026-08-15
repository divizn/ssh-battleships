#!/bin/bash
set -euxo pipefail

dnf install -y docker git

install -m 0755 -d /usr/libexec/docker/cli-plugins
curl -fsSL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 \
  -o /usr/libexec/docker/cli-plugins/docker-compose
chmod +x /usr/libexec/docker/cli-plugins/docker-compose

systemctl enable --now docker

# session manager is the way in, so nothing here needs sshd and the game can have port 22
systemctl disable --now sshd

# 512MB runs the game but will not compile it
dd if=/dev/zero of=/swapfile bs=1M count=2048
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

install -m 700 -d /root/.ssh

cat > /usr/local/bin/${name}-deploy <<'DEPLOY'
#!/bin/bash
set -euo pipefail

app=/srv/${name}

aws ssm get-parameter --name /${name}/deploy-key --with-decryption \
  --query Parameter.Value --output text > /root/.ssh/deploy_key
chmod 600 /root/.ssh/deploy_key
ssh-keyscan -t ed25519 github.com > /root/.ssh/known_hosts
export GIT_SSH_COMMAND="ssh -i /root/.ssh/deploy_key -o IdentitiesOnly=yes"

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
