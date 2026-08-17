#!/usr/bin/env bash
#
# Installs the current working tree on a test host.
#
# This is the loop for trying a change on a real VPS before committing it: the
# working tree is archived, copied over, and handed to install.sh, which builds
# the image on the host and reconciles the running stack.
#
# The working tree is deliberate. `git archive HEAD` would ship the last commit
# instead, which rebuilds code that has not changed and looks exactly like a
# deploy that did nothing — the failure mode this script exists to avoid.
#
# Usage:
#   scripts/deploy-test.sh ubuntu@host
#   DEPLOY_TEST_HOST=ubuntu@host scripts/deploy-test.sh
#
# Anything install.sh honours can be set in the environment and is forwarded:
#   STACKER_HOST=stacker.example.com scripts/deploy-test.sh ubuntu@host

set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The remote directory is created under this prefix and later removed with sudo,
# so the prefix is checked before anything is deleted.
readonly REMOTE_PREFIX="/tmp/stacker-deploy-test"

# Installer settings worth carrying to the host. ssh does not forward the
# environment, so each has to be passed explicitly; only the ones actually set
# locally are sent, leaving the installer's own defaults alone.
readonly FORWARDED_VARS=(
  STACKER_HOST
  STACKER_STACK_NAME
  STACKER_IMAGE
  STACKER_VERSION
  PUBLIC_IP
  ADVERTISE_ADDR
)

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

HOST="${1:-${DEPLOY_TEST_HOST:-}}"
[ -n "$HOST" ] || die "pass the host as an argument or set DEPLOY_TEST_HOST (example: ubuntu@192.0.2.10)"

for tool in git ssh scp tar; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

archive=""
remote_dir=""
cleanup() {
  [ -n "$archive" ] && rm -f "$archive"
  # The remote copy holds the whole source tree; it goes whether or not the
  # install succeeded. Failure to clean up is reported but never masks the
  # install's own exit status.
  if [ -n "$remote_dir" ]; then
    ssh "$HOST" "sudo rm -rf '$remote_dir'" || printf 'warning: could not remove %s on %s\n' "$remote_dir" "$HOST" >&2
  fi
}
trap cleanup EXIT

# The tree is assembled in a throwaway index so `git archive` still does the
# work: it handles deletions, respects .gitignore — no node_modules, no build
# output — and copes with any filename, none of which a hand-rolled file list
# does well. GIT_INDEX_FILE keeps the real index untouched, so a staged change
# is never disturbed.
log "Archiving the working tree"
archive="$(mktemp -t stacker-deploy-test.XXXXXX.tar.gz)"
index="$(mktemp -t stacker-deploy-index.XXXXXX)"
(
  cd "$ROOT"
  export GIT_INDEX_FILE="$index"
  git read-tree HEAD
  git add -A .
  tree="$(git write-tree)"
  git archive --format=tar.gz --output="$archive" "$tree"
)
rm -f "$index"

# A tree with no install.sh would fail confusingly on the host, several minutes
# and one image build later.
tar -tzf "$archive" | grep -qx "install.sh" || die "install.sh is missing from the archive"

log "Copying to $HOST"
remote_dir="$(ssh "$HOST" "mktemp -d ${REMOTE_PREFIX}.XXXXXX")"
case "$remote_dir" in
  "$REMOTE_PREFIX".*) ;;
  *) remote_dir=""; die "unexpected remote directory: $remote_dir" ;;
esac
scp -q "$archive" "$HOST:$remote_dir/repo.tar.gz"

# Built on the host from the copied source: STACKER_SOURCE_DIR is what tells
# install.sh to use it instead of cloning the repository itself.
env_prefix="STACKER_SOURCE_DIR='$remote_dir'"
for name in "${FORWARDED_VARS[@]}"; do
  value="${!name:-}"
  [ -n "$value" ] || continue
  env_prefix+=" $name='$value'"
  log "Forwarding $name=$value"
done

log "Running the installer on $HOST"
ssh -t "$HOST" "
  set -Eeuo pipefail
  tar -xzf '$remote_dir/repo.tar.gz' -C '$remote_dir'
  if [ \"\$(id -u)\" -eq 0 ]; then
    $env_prefix bash '$remote_dir/install.sh'
  else
    sudo $env_prefix bash '$remote_dir/install.sh'
  fi
"

log "Done"
