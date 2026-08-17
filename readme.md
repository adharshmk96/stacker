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

## Projects

A project is one application. Each of its environments (`production`,
`staging`, …) deploys the same source as its own Swarm stack, with its own
variables, secrets, hostnames and rollout settings.

Deploying runs entirely on the machine Stacker is installed on: it clones the
repository into a throwaway workspace, builds whatever the Compose file builds,
deploys the result with `docker stack deploy`, writes the environment's
hostnames into Traefik's dynamic configuration, and deletes the workspace.
Projects and Deployments both report what Docker is actually running, polled
live, and a run's log can be followed while it happens.

Two things to know:

- Only Docker Compose files are supported as the source, deployed as Swarm
  stacks. Replicas and placement are applied to every service that does not
  pin its own in the Compose file.
- Images built from a repository exist only on the node that built them, so a
  service with a `build:` section is scheduled on the Stacker host. Services
  that pull a published image schedule anywhere in the Swarm.

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
