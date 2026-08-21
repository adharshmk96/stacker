#!/usr/bin/env bash

set -Eeuo pipefail

readonly REPOSITORY_URL="${STACKER_REPOSITORY_URL:-https://github.com/adharshmk96/stacker.git}"
readonly REPOSITORY_REF="${STACKER_VERSION:-main}"
readonly STACK_NAME="${STACKER_STACK_NAME:-stacker}"
readonly IMAGE="${STACKER_IMAGE:-stacker:local}"
readonly SCRIPT_PATH="${BASH_SOURCE[0]:-}"
readonly SCRIPT_DIR="$(
  if [ -n "$SCRIPT_PATH" ]; then
    cd "$(dirname "$SCRIPT_PATH")" 2>/dev/null && pwd || true
  fi
)"

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

require_linux_root() {
  [ "$(uname -s)" = "Linux" ] || die "this installer supports Linux VPS hosts only"
  [ "$(id -u)" -eq 0 ] || die "run this installer as root (for example: curl ... | sudo bash)"
}

install_base_tools() {
  local missing=()
  local tool
  for tool in curl git sed awk hostname ip; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
  done
  [ "${#missing[@]}" -eq 0 ] && return

  log "Installing required tools: ${missing[*]}"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq </dev/null
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl git sed gawk hostname iproute2 ca-certificates </dev/null
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q curl git sed gawk hostname iproute ca-certificates </dev/null
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q curl git sed gawk hostname iproute ca-certificates </dev/null
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache curl git sed gawk hostname iproute2 ca-certificates </dev/null
  else
    die "install curl, git, sed, awk, hostname, iproute2 and CA certificates, then rerun"
  fi
}

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    log "Installing Docker Engine"
    curl -fsSL https://get.docker.com | sh
  fi
  if ! docker info >/dev/null 2>&1 && command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now docker </dev/null
  fi
  docker info >/dev/null 2>&1 || die "Docker is installed but its daemon is unavailable"
}

local_ip() {
  if [ -n "${ADVERTISE_ADDR:-}" ]; then
    printf '%s\n' "$ADVERTISE_ADDR"
    return
  fi

  local address
  address="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{ for (i=1; i<=NF; i++) if ($i == "src") { print $(i+1); exit } }')"
  [ -n "$address" ] || address="$(hostname -I 2>/dev/null | awk '{print $1}')"
  [ -n "$address" ] || die "could not detect the swarm advertise address; rerun with ADVERTISE_ADDR=x.x.x.x"
  printf '%s\n' "$address"
}

public_ip() {
  if [ -n "${PUBLIC_IP:-}" ]; then
    printf '%s\n' "$PUBLIC_IP"
    return
  fi

  local address
  address="$(curl -4fsS --max-time 8 https://1.1.1.1/cdn-cgi/trace 2>/dev/null | awk -F= '$1 == "ip" {print $2; exit}')"
  [ -n "$address" ] || address="$(curl -4fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)"
  [[ "$address" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || die "could not detect the public IPv4 address; rerun with PUBLIC_IP=x.x.x.x"
  printf '%s\n' "$address"
}

installed_stacker_host() {
  docker volume inspect stacker-traefik-config >/dev/null 2>&1 || return 0

  docker run --rm \
    -v stacker-traefik-config:/config:ro \
    alpine:3.23 sh -c 'cat /config/dynamic/stacker.yml 2>/dev/null' 2>/dev/null \
    | sed -n 's/.*Host(`\([^`]*\)`).*/\1/p' \
    | head -n 1
}

ensure_swarm_manager() {
  local address="$1"
  local state control
  state="$(docker info --format '{{.Swarm.LocalNodeState}}')"
  control="$(docker info --format '{{.Swarm.ControlAvailable}}')"

  if [ "$state" = "inactive" ]; then
    log "Initialising Docker Swarm on $address"
    docker swarm init --advertise-addr "$address" </dev/null
  fi

  state="$(docker info --format '{{.Swarm.LocalNodeState}}')"
  control="$(docker info --format '{{.Swarm.ControlAvailable}}')"
  [ "$state" = "active" ] || die "Docker Swarm is not active on the current node (state: $state)"
  [ "$control" = "true" ] || die "the current node belongs to a swarm but is not a manager"
}

resolve_source() {
  if [ -f "${STACKER_SOURCE_DIR:-}/Dockerfile" ] && [ -f "${STACKER_SOURCE_DIR:-}/deploy/stack.yml" ]; then
    SOURCE_DIR="$STACKER_SOURCE_DIR"
    return
  fi
  if [ -f "$SCRIPT_DIR/Dockerfile" ] && [ -f "$SCRIPT_DIR/deploy/stack.yml" ]; then
    SOURCE_DIR="$SCRIPT_DIR"
    return
  fi

  SOURCE_TMP="$(mktemp -d)"
  log "Cloning Stacker ${REPOSITORY_REF}"
  git clone --quiet --depth 1 --branch "$REPOSITORY_REF" "$REPOSITORY_URL" "$SOURCE_TMP" </dev/null
  SOURCE_DIR="$SOURCE_TMP"
}

populate_traefik_config() {
  local source_dir="$1" host="$2"
  RENDER_TMP="$(mktemp -d)"
  docker volume create stacker-traefik-config </dev/null >/dev/null
  docker volume create stacker-traefik-data </dev/null >/dev/null
  docker volume create stacker-data </dev/null >/dev/null

  # The Traefik config volume is the source of truth for configured domains and
  # project routers. Never replace it on an update: doing so would erase both
  # dashboard changes and unrelated Traefik configuration.
  if docker run --rm \
    -v stacker-traefik-config:/target:ro \
    alpine:3.23 sh -c 'find /target -mindepth 1 -print -quit | grep -q .' </dev/null; then
    log "Keeping existing Traefik configuration"
  else
    mkdir -p "$RENDER_TMP/dynamic"
    cp "$source_dir/deploy/traefik/traefik.yml" "$RENDER_TMP/traefik.yml"
    sed "s/__STACKER_HOST__/$host/g" "$source_dir/deploy/traefik/dynamic/stacker.yml" > "$RENDER_TMP/dynamic/stacker.yml"

    log "Initialising Traefik configuration"
    docker run --rm \
      -v stacker-traefik-config:/target \
      -v "$RENDER_TMP:/source:ro" \
      alpine:3.23 sh -c 'cp -R /source/. /target/' </dev/null
  fi

  # The work root is a host directory rather than a volume on purpose: swarm
  # resolves a stack's relative bind mounts on the host, so a deploy's checkout
  # has to exist at the same path outside the stacker container as inside it.
  mkdir -p /var/lib/stacker/workspaces
  chmod 700 /var/lib/stacker/workspaces
}

deploy() {
  local source_dir="$1" advertise_addr="$2" host="$3"
  local built_at node_name stack_file
  built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  node_name="$(hostname -f 2>/dev/null || hostname)"
  stack_file="$RENDER_TMP/stack.yml"

  log "Building $IMAGE"
  docker build --pull \
    --build-arg "STACKER_VERSION=$REPOSITORY_REF" \
    --build-arg "STACKER_BUILT_AT=$built_at" \
    -t "$IMAGE" "$source_dir" </dev/null

  sed \
    -e "s|__STACKER_IMAGE__|$IMAGE|g" \
    -e "s|__STACKER_NODE_NAME__|$node_name|g" \
    -e "s|__STACKER_ADVERTISE_ADDR__|$advertise_addr|g" \
    -e "s|__STACKER_STACK_NAME__|$STACK_NAME|g" \
    "$source_dir/deploy/stack.yml" > "$stack_file"

  log "Deploying $STACK_NAME"
  docker stack deploy --detach=true --resolve-image never -c "$stack_file" "$STACK_NAME" </dev/null

  # Local image tags and named-volume config do not alter the service specs.
  # Force both services so rerunning the installer always applies the new image
  # and Traefik files.
  docker service update --force --detach=true "${STACK_NAME}_stacker" </dev/null >/dev/null
  docker service update --force --detach=true "${STACK_NAME}_traefik" </dev/null >/dev/null
}

main() {
  trap 'rm -rf "${SOURCE_TMP:-}" "${RENDER_TMP:-}"' EXIT
  require_linux_root
  install_base_tools
  ensure_docker

  local advertise_addr host installed_host
  advertise_addr="$(local_ip)"
  installed_host="$(installed_stacker_host)"

  if [ -n "${STACKER_HOST:-}" ]; then
    host="$STACKER_HOST"
  elif [ -n "$installed_host" ]; then
    host="$installed_host"
    log "Preserving configured domain: $installed_host"
  else
    host="stacker.$(public_ip).sslip.io"
  fi

  [[ "$STACK_NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]] || die "STACKER_STACK_NAME contains invalid characters"
  [[ "$host" =~ ^[a-zA-Z0-9.-]+$ ]] || die "STACKER_HOST must be a DNS hostname"
  [[ "$advertise_addr" =~ ^[a-zA-Z0-9.:_-]+$ ]] || die "ADVERTISE_ADDR contains invalid characters"
  [[ "$IMAGE" =~ ^[a-zA-Z0-9./:_-]+$ ]] || die "STACKER_IMAGE contains invalid characters"

  ensure_swarm_manager "$advertise_addr"
  resolve_source
  populate_traefik_config "$SOURCE_DIR" "$host"
  deploy "$SOURCE_DIR" "$advertise_addr" "$host"

  log "Stacker is installed: https://$host"
  log "Rerun this installer at any time to reconcile or upgrade the deployment."
}

main "$@"
