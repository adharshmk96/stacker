#!/usr/bin/env bash

set -Eeuo pipefail

readonly STACK_NAME="${STACKER_STACK_NAME:-stacker}"
readonly IMAGE="${STACKER_IMAGE:-stacker:local}"
readonly VOLUMES=(stacker-data stacker-traefik-config stacker-traefik-data)

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

require_linux_root() {
  [ "$(uname -s)" = "Linux" ] || die "this uninstaller supports Linux VPS hosts only"
  [ "$(id -u)" -eq 0 ] || die "run this uninstaller as root (for example: curl ... | sudo bash)"
}

remove_stack() {
  local state control waited=0
  state="$(docker info --format '{{.Swarm.LocalNodeState}}' 2>/dev/null || true)"
  control="$(docker info --format '{{.Swarm.ControlAvailable}}' 2>/dev/null || true)"

  if [ "$state" != "active" ] || [ "$control" != "true" ]; then
    log "Docker Swarm is not an active manager; no stack can be removed"
    return
  fi

  if ! docker stack services "$STACK_NAME" >/dev/null 2>&1; then
    log "Stack $STACK_NAME is already absent"
    return
  fi

  log "Removing stack $STACK_NAME"
  docker stack rm "$STACK_NAME" >/dev/null

  while docker service ls \
    --filter "label=com.docker.stack.namespace=$STACK_NAME" \
    --format '{{.ID}}' | grep -q .; do
    [ "$waited" -lt 60 ] || die "timed out waiting for stack $STACK_NAME to stop"
    sleep 2
    waited=$((waited + 2))
  done
}

remove_volumes() {
  local volume
  for volume in "${VOLUMES[@]}"; do
    if docker volume inspect "$volume" >/dev/null 2>&1; then
      log "Removing volume $volume"
      docker volume rm "$volume" >/dev/null || die "could not remove volume $volume; it may still be in use"
    fi
  done
}

remove_image() {
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    log "Removing image $IMAGE"
    docker image rm "$IMAGE" >/dev/null || die "could not remove image $IMAGE; it may still be in use"
  fi
}

main() {
  require_linux_root
  command -v docker >/dev/null 2>&1 || {
    log "Docker is absent; Stacker is already uninstalled"
    return
  }
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable"

  [[ "$STACK_NAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]] || die "STACKER_STACK_NAME contains invalid characters"
  [[ "$IMAGE" =~ ^[a-zA-Z0-9./:_-]+$ ]] || die "STACKER_IMAGE contains invalid characters"

  remove_stack
  remove_volumes
  remove_image

  log "Stacker is completely removed. Docker and Docker Swarm were preserved."
}

main "$@"
