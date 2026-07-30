# Picket

Picket is a lightweight server monitoring dashboard with a hub and agents. It was forked from [Beszel](https://github.com/henrygd/beszel).

## Features

- CPU, memory, disk, network, load, GPU, and uptime metrics
- Docker and Podman container monitoring
- Per-system alert thresholds
- Telegram alerts for an allowed list of users
- Native Linux agent installation with systemd
- One-time SSH access from a copied terminal command

## Run the Hub

Run the published image:

```sh
docker run -d \
  --name picket \
  -p 8090:8090 \
  -e PICKET_HUB_PASSWORD=change-me \
  -v picket_data:/picket_data \
  ghcr.io/bgwastu/picket:latest
```

Then open `http://127.0.0.1:8090` and sign in with that password.

For a local binary build:

```sh
PICKET_HUB_PASSWORD=change-me ./picket-hub serve
```

The image is published to GitHub Container Registry on every version tag.

## Install an Agent

The system page provides a one-line installer. Run it on the Linux host that
should be monitored:

```sh
curl -fsSL 'https://picket.example.com/api/picket/agent-install/TOKEN' | sudo sh
```

The token is generated for the system and is used to authorize the download.
Set `APP_URL` when the hub is behind a reverse proxy; otherwise the installer
uses the public host and scheme from the request URL.

## Non-interactive SSH

The SSH command generated from a system page can also be used by automation or
an AI agent without installing a Picket client. Pass launcher options after
`sh -s --`:

```sh
curl -fsSL 'https://picket.example.com/api/picket/ssh-launch/TOKEN' | \
  sh -s -- --non-interactive --identity "$HOME/.ssh/agent_key" -- uname -a
```

Non-interactive mode enables `BatchMode=yes` and strict host-key checking. The
host key must already exist in `~/.ssh/known_hosts`, or in the file named by
`SSH_KNOWN_HOSTS`. The launch token is short-lived and the SSH tunnel closes
after 15 minutes without traffic.

Picket is released under the MIT License.
