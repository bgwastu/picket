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
