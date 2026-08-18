# Stacker

Stacker manages application deployments with Docker Swarm.

## Install on a VPS

Run as root on a clean Linux VPS:

```sh
curl -fsSL https://raw.githubusercontent.com/adharshmk96/stacker/main/install.sh | sudo bash
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

## Preparing an app to deploy

An app needs a Dockerfile and a Compose file in its repository, and nothing
else. Stacker never edits either one: it writes a second Compose file next to
yours and passes both to `docker stack deploy`, so what runs in production is
your file plus a readable overlay printed in the run's log.

A minimal app:

```
my-app/
  Dockerfile
  docker-compose.yml
```

```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY . .
EXPOSE 3000
CMD ["node", "server.js"]
```

```yaml
services:
  web:
    build: .
    environment:
      PORT: 3000

  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:
```

Create the project pointing at the repository, set `composePath` if the file is
not `docker-compose.yml` at the root, then add an environment and give it a
domain of host `app.example.com` → service `web`, port `3000`. That is the whole
setup: the next deploy builds `web`, starts `db`, and publishes the hostname.

### What Stacker adds for you

- **The environment's variables and secrets** are injected into every service and
  are available for `${VAR}` interpolation in the Compose file itself — which is
  how `${DB_PASSWORD}` above gets its value. Secrets win a key collision with a
  variable of the same name, and are masked in deploy logs.
- **An image name** for any service with a `build:` and no `image:`.
- **The proxy network**, joined only by services a domain points at. Traefik
  reaches them over it, so you do **not** publish `ports:` for anything a domain
  routes to. Publish a host port only for something genuinely outside the proxy.
- **Rollout settings** — replicas, placement, update order, health grace period,
  auto-rollback — applied to every service that does not already set its own
  `deploy.replicas` or `deploy.placement.constraints`. Pin either in your file
  and Stacker leaves that service alone.

### Rules to write to

- **Services talk to each other by name.** `db` above is reachable at `db:5432`
  from `web`, on the stack's default overlay network.
- **The routed port is the container's port**, not a published one. Traefik
  forwards to `service:port` inside the network.
- **Swarm ignores parts of Compose** it has no equivalent for — `depends_on`,
  `restart`, `container_name`, `profiles`, `build` at deploy time. Use
  `deploy.restart_policy` instead of `restart`, and make services tolerate
  starting in any order rather than relying on `depends_on`.
- **Relative paths resolve against the Compose file's directory**, as usual —
  build contexts, `env_file`, bind mounts. The repository is cloned intact, so
  they work the way they do locally.
- **A built image lives only on the Stacker host.** Services with a `build:` are
  scheduled there. Services that pull a published image schedule anywhere in the
  Swarm.
- **Named volumes are per-node.** A stateful service pinned to one node keeps its
  data; moved to another it starts empty. Pin it with
  `deploy.placement.constraints`, or point it at managed storage.
- **Migrations belong in the app's startup**, or in a one-shot service, not in a
  Compose lifecycle hook Swarm will not run.

### Deploying

Deploys are triggered manually, by a push to the environment's branch, by a tag
matching a pattern, or on a schedule. Each environment of a project deploys the
same source as its own Swarm stack, named `stk-<project>-<environment>`, with its
own variables, secrets, domains and rollout settings — so `staging` and
`production` differ only in configuration.

Turn on **always pull** for an environment whose images come from a registry, or
whose Dockerfile has a floating base like `FROM node:22`, so a rebuild picks up
the moved tag instead of a stale local layer.


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
