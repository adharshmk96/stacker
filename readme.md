# Stacker

Stacker manages application deployments with Docker Swarm.

## Install on a VPS

Run as root on a clean Linux VPS:

```sh
curl -fsSL https://raw.githubusercontent.com/adharshmk96/stacker/main/install.sh | bash
```

The installer adds Docker when needed, initialises Swarm, builds Stacker from
source, and deploys Stacker plus Traefik. It prints the generated HTTPS URL.
Rerunning the same command safely reconciles and upgrades the installation.

Useful overrides:

```sh
PUBLIC_IP=203.0.113.10 \
ADVERTISE_ADDR=10.0.0.10 \
STACKER_HOST=stacker.example.com \
STACKER_VERSION=main \
bash install.sh
```

Persistent state lives in the `stacker-data`, `stacker-traefik-config`, and
`stacker-traefik-data` Docker volumes. The Traefik volume contains
`/etc/traefik/traefik.yml` and `/etc/traefik/dynamic/stacker.yml`.

## Uninstall

Remove Stacker, Traefik, the local image, and all persistent Stacker data:

```sh
curl -fsSL https://raw.githubusercontent.com/adharshmk96/stacker/main/uninstall.sh | sudo bash
```

Docker and Docker Swarm are preserved because other applications may use them.

## Local Docker development

```sh
docker compose up --build
```

Open `http://localhost:8080`. The production installer is Linux-only because
it configures the host's Docker Swarm.
