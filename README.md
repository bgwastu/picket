# Picket

Picket is a lightweight server monitoring dashboard with a hub and agents. It was forked from [Beszel](https://github.com/henrygd/beszel).

## Features

- CPU, memory, disk, network, load, GPU, and uptime metrics
- Docker and Podman container monitoring
- Per-system alert thresholds
- Telegram alerts for an allowed list of users
- Native Linux agent installation with systemd

## Run the Hub

```sh
PICKET_HUB_PASSWORD=change-me ./picket-hub serve
```

Open `http://127.0.0.1:8090` and sign in with that password.

Picket is released under the MIT License.
