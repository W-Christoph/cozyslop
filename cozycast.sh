#!/usr/bin/env bash
# Everyday commands for a CozyCast server, and for a computer that runs a
# room for a server elsewhere (docs/home-hosting.md). Without arguments it
# lists them.
#
# A checkout is one or the other: "setup" makes it a server (.env),
# "connect" a room for another server (.env.node).
set -euo pipefail
cd "$(dirname "$0")"

usage() {
  cat <<'EOF'
On the server:
  ./cozycast.sh setup            ask a few questions and write .env
  ./cozycast.sh start            build and start (also after changing .env)
  ./cozycast.sh stop
  ./cozycast.sh update           git pull, then start
  ./cozycast.sh status
  ./cozycast.sh logs [service]   follow the log (default: server)

On a computer that runs a room for a server elsewhere:
  ./cozycast.sh connect <server> start the room and show the code to accept
                                 under Admin > Rooms, e.g. connect cozy.example.com
  ./cozycast.sh start | stop | update | status
  ./cozycast.sh logs [service]   default: agent
  ./cozycast.sh forget           stop and delete the pairing and the room's files
EOF
}

die() { echo "$*" >&2; exit 1; }

# The room for another server is its own compose project with its own
# settings file, so both kinds of commands can share this script.
node() { docker compose --env-file .env.node -f compose.node.yaml "$@"; }

ROLE=
find_role() {
  if [ -f .env.node ] && [ ! -f .env ]; then ROLE=node
  elif [ -f .env ]; then ROLE=server
  else die "Not set up yet: run \"$0 setup\" on a server, or \"$0 connect <server>\" for a room."
  fi
}

setting() { sed -n "s/^$1=//p" .env | tail -n 1; }

# The server's own room, when it only runs on demand, is in a profile that
# plain "docker compose" commands leave alone.
on_demand() { case "$(setting COMPOSE_FILE)" in *room-on-demand*) return 0 ;; *) return 1 ;; esac; }

ask() { # ask <question> <default>
  local answer
  read -r -p "$1 [$2]: " answer || true
  echo "${answer:-$2}"
}

setup() {
  [ ! -f .env ] || die ".env exists already. Edit it, or move it away to start over."
  [ ! -f .env.node ] || die "This checkout runs a room for another server (.env.node)."
  command -v openssl >/dev/null || die "openssl is needed to make the secrets."

  local ip domain hosting room files=compose.yaml gid=""
  ip=$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' || true)
  ip=$(ask "Public IP address of this server" "${ip:-}")
  [ -n "$ip" ] || die "The server needs its public IP address."
  domain=$(ask "Domain name for HTTPS (empty: plain HTTP)" "")
  hosting=$(ask "Rooms on other computers (home hosting)? y/n" "n")
  echo "This server's own room:"
  echo "  1) always running"
  echo "  2) stopped until an admin starts it under Admin > Rooms"
  echo "  3) none"
  room=$(ask "Which" "1")

  case "$hosting" in y* | Y*) files+=:compose.tunnel.yaml ;; esac
  case "$room" in
    1) ;;
    2)
      files+=:compose.room-on-demand.yaml
      gid=$(stat -c %g /var/run/docker.sock 2>/dev/null) || die "Docker's socket /var/run/docker.sock was not found."
      ;;
    3) files+=:compose.no-room.yaml ;;
    *) die "Answer 1, 2 or 3." ;;
  esac
  [ ! -f compose.override.yaml ] || files+=:compose.override.yaml

  local password
  password=$(openssl rand -base64 15 | tr -d '/+=')
  (
    umask 077
    {
      echo "PUBLIC_IP=$ip"
      echo "NEKO_API_TOKEN=$(openssl rand -hex 32)"
      echo "ADMIN_PASSWORD=$password"
      [ -z "$domain" ] || echo "DOMAIN=$domain"
      [ "$files" = compose.yaml ] || echo "COMPOSE_FILE=$files"
      [ -z "$gid" ] || echo "DOCKER_GID=$gid"
    } > .env
  )
  echo
  echo "Wrote .env. Log in as \"admin\" with the password $password"
  echo "(it stays in .env as ADMIN_PASSWORD). More settings: .env.example."
  echo "Ports to open: 80 and 443 (TCP)$(
    [ "$room" = 3 ] || printf ', 52000 (UDP and TCP)'
    case "$files" in *tunnel*) printf ', 51820 (UDP), 52099 (UDP and TCP)' ;; esac
  )."
  echo "Next: $0 start"
}

start() {
  find_role
  if [ "$ROLE" = node ]; then
    node up -d --build
    return
  fi
  docker compose up -d --build
  if on_demand; then
    # Built and created, not started: that is the admin's button.
    docker compose --profile room-default create --build room-default
    echo "The server's room is stopped; start it under Admin > Rooms."
  fi
}

stop() {
  find_role
  if [ "$ROLE" = node ]; then
    node stop
  elif on_demand; then
    docker compose --profile room-default stop
  else
    docker compose stop
  fi
}

status() {
  find_role
  if [ "$ROLE" = node ]; then
    echo "Room for $(sed -n 's/^COZYCAST_HUB=//p' .env.node):"
    node ps -a
  elif on_demand; then
    docker compose --profile room-default ps -a
  else
    docker compose ps -a
  fi
}

logs() {
  find_role
  if [ "$ROLE" = node ]; then
    node logs -f --tail 100 "${1:-agent}"
  elif on_demand; then
    docker compose --profile room-default logs -f --tail 100 "${1:-server}"
  else
    docker compose logs -f --tail 100 "${1:-server}"
  fi
}

update() {
  find_role
  git pull --ff-only
  start
}

connect() {
  [ $# -eq 1 ] || die "Which server? For example: $0 connect cozy.example.com"
  [ ! -f .env ] || die "This checkout is a server (.env); use another one for a room."
  if [ -f .env.node ] && [ "$(sed -n 's/^COZYCAST_HUB=//p' .env.node)" != "$1" ]; then
    die "This computer is set up for $(sed -n 's/^COZYCAST_HUB=//p' .env.node). \"$0 forget\" first to change servers."
  fi
  echo "COZYCAST_HUB=$1" > .env.node
  node up -d --build
  echo
  echo "Below is the agent's log. The first time it shows a code: compare it"
  echo "under Admin > Rooms on $1 and accept. Ctrl+C stops watching; the room keeps running."
  echo
  node logs -f --tail 20 agent
}

forget() {
  [ -f .env.node ] || die "This checkout runs no room for another server."
  local answer
  read -r -p "Delete the pairing and everything in the room's home folder? Type yes: " answer || true
  [ "$answer" = yes ] || die "Nothing deleted."
  node down -v
  rm -f .env.node
  echo "Deleted. On the server, remove the room under Admin > Rooms."
}

case "${1:-}" in
  setup | start | stop | status | update | forget) [ $# -eq 1 ] || die "$1 takes no arguments."; "$1" ;;
  logs) shift; logs "$@" ;;
  connect) shift; connect "$@" ;;
  "" | help | -h | --help) usage ;;
  *) usage >&2; exit 1 ;;
esac
