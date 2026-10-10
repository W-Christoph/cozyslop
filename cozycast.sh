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
                                 under Admin > Rooms, e.g. connect cozy.example.com;
                                 the first time it asks what the room starts
                                 with and writes .env.node
  ./cozycast.sh start | stop | update | status   (start also after changing .env.node)
  ./cozycast.sh logs [service]   default: agent
  ./cozycast.sh forget           stop and delete the pairing; asks about the
                                 room's files separately

Moving a room's desktop (its files and Firefox profile) to another computer:
  ./cozycast.sh export-room [file]  where the room runs now: write
                                    cozycast-room.tar.gz (or the file named)
  ./cozycast.sh import-room <file>  where it runs next, once the file is copied
                                    there: make the room's home folder from it

Moving the server (accounts, chat, room settings, pairings) to another server:
  ./cozycast.sh export-server [file]  on the old server, which keeps running:
                                      write cozycast-server.tar.gz (or the file named)
  ./cozycast.sh import-server <file>  on the new one, after "setup": replace
                                      what it knows by the file's content
EOF
}

die() { echo "$*" >&2; exit 1; }

# The room for another server is its own compose project with its own
# settings file, so both kinds of commands can share this script.
node() {
  if [ -f .env.node ]; then
    docker compose --env-file .env.node -f compose.node.yaml "$@"
  else
    docker compose -f compose.node.yaml "$@"
  fi
}

hub() { [ ! -f .env.node ] || sed -n 's/^COZYCAST_HUB=//p' .env.node | tail -n 1; }

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

sure() { # sure <question>: only "yes" goes on
  local answer
  read -r -p "$1 Type yes: " answer || true
  [ "$answer" = yes ]
}

# set_env <file> <name> <value>: the file's line for <name>, commented out or
# not, becomes the setting; without such a line it is added at the end.
set_env() {
  [ -f "$1" ] || : > "$1"
  name=$2 value=$3 awk '
    !done && $0 ~ "^#? ?" ENVIRON["name"] "=" { print ENVIRON["name"] "=" ENVIRON["value"]; done = 1; next }
    { print }
    END { if (!done) print ENVIRON["name"] "=" ENVIRON["value"] }' "$1" > "$1.tmp"
  mv "$1.tmp" "$1"
}

# What a room starts with: its desktop and its stream, until the room's
# settings pick something else. These set the caller's screen, bitrate,
# scale and preset.
defaults() { screen=1280x720@30 bitrate=2500 scale=100 preset=veryfast; }

ask_defaults() { # ask_defaults <whose room>
  echo "$1 starts with (Enter keeps a default; the room's settings change it later):"
  screen=$(ask "Desktop size and frame rate" "$screen")
  [[ $screen =~ ^[1-9][0-9]*x[1-9][0-9]*@[1-9][0-9]*$ ]] || die "A desktop size looks like 1280x720@30."
  bitrate=$(ask "Stream bitrate in kbit/s" "$bitrate")
  [[ $bitrate =~ ^[1-9][0-9]*$ ]] || die "The bitrate is a number, like 2500."
  scale=$(ask "Stream size in percent of the desktop size" "$scale")
  [[ $scale =~ ^[1-9][0-9]?$|^100$ ]] || die "The stream size is a number from 1 to 100."
  preset=$(ask "Encoder speed: ultrafast, superfast or veryfast (faster needs less CPU and looks blockier)" "$preset")
  case "$preset" in
    ultrafast | superfast | veryfast | faster | fast | medium) ;;
    *) die "The encoder speed is one of x264's presets, like veryfast." ;;
  esac
}

# Only what differs is written: the rest stays commented out, as the default.
save_defaults() { # save_defaults <file>
  [ "$screen" = 1280x720@30 ] || set_env "$1" SCREEN "$screen"
  [ "$bitrate" = 2500 ] || set_env "$1" STREAM_BITRATE "$bitrate"
  [ "$scale" = 100 ] || set_env "$1" STREAM_SCALE "$scale"
  [ "$preset" = veryfast ] || set_env "$1" X264_PRESET "$preset"
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

  local screen bitrate scale preset
  defaults
  [ "$room" = 3 ] || ask_defaults "This server's room"

  # A copy of .env.example, so that every other setting is there to see,
  # commented out with its default.
  local password
  password=$(openssl rand -base64 15 | tr -d '/+=')
  (
    umask 077
    cp .env.example .env.new
    set_env .env.new PUBLIC_IP "$ip"
    set_env .env.new NEKO_API_TOKEN "$(openssl rand -hex 32)"
    set_env .env.new ADMIN_PASSWORD "$password"
    [ -z "$domain" ] || set_env .env.new DOMAIN "$domain"
    [ "$files" = compose.yaml ] || set_env .env.new COMPOSE_FILE "$files"
    [ -z "$gid" ] || set_env .env.new DOCKER_GID "$gid"
    save_defaults .env.new
    mv .env.new .env
  )
  echo
  echo "Wrote .env. Log in as \"admin\" with the password $password"
  echo "(it stays in .env as ADMIN_PASSWORD). The other settings are in .env"
  echo "too, commented out with their defaults."
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
    # Built and created, not started: that is the admin's button. A room
    # that runs is left running.
    docker compose --profile room-default create --build room-default
    echo "The server's own room is started and stopped under Admin > Rooms."
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
    echo "Room for $(hub):"
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

# The Docker volume behind a compose volume, if there is one yet.
volume() { # volume <node | room_compose> <its name in the compose file>
  local project
  project=$("$1" config | sed -n 's/^name: //p') || return
  docker volume ls -q --filter "label=com.docker.compose.project=$project" --filter "label=com.docker.compose.volume=$2"
}

# Forgets the server: the agent's keys and what it handed the room. The
# room's home folder stays.
unpair() {
  local key name
  node down
  for key in state room-env; do
    name=$(volume node "$key")
    [ -z "$name" ] || docker volume rm "$name" > /dev/null
  done
}

# A question of its own, after the pairing is gone: the desktop can go on
# to the next server.
delete_home() {
  local name
  name=$(volume node home)
  [ -n "$name" ] || return 0
  if sure "Also delete the room's files (its home folder, with Firefox's logins)?"; then
    docker volume rm "$name" > /dev/null
    echo "Deleted the room's files."
  else
    echo "Kept the room's files: the next room on this computer starts with them."
  fi
}

connect() {
  [ $# -eq 1 ] || die "Which server? For example: $0 connect cozy.example.com"
  [ ! -f .env ] || die "This checkout is a server (.env); use another one for a room."
  local old
  old=$(hub)
  if [ -n "$old" ] && [ "$old" != "$1" ]; then
    echo "This computer runs a room for $old."
    sure "Delete that pairing and connect to $1 instead?" || die "Nothing changed."
    unpair
    echo "Pairing deleted. On $old, remove the room under Admin > Rooms."
    delete_home
  fi
  if [ ! -f .env.node ]; then
    # The first time: this computer decides what its room starts with.
    local screen bitrate scale preset
    defaults
    ask_defaults "The room on this computer"
    cp .env.node.example .env.node.new
    save_defaults .env.node.new
    mv .env.node.new .env.node
  fi
  set_env .env.node COZYCAST_HUB "$1"
  node up -d --build
  echo
  echo "Below is the agent's log. The first time it shows a code: compare it"
  echo "under Admin > Rooms on $1 and accept. Ctrl+C stops watching; the room keeps running."
  echo
  node logs -f --tail 20 agent
}

forget() {
  if [ -f .env.node ]; then
    sure "Delete the pairing with $(hub)?" || die "Nothing deleted."
    unpair
    rm -f .env.node
    echo "Pairing deleted. On the server, remove the room under Admin > Rooms."
  elif [ -f .env ] || [ -z "$(volume node home)" ]; then
    die "This checkout runs no room for another server."
  fi
  delete_home
}

# This checkout's room: the server's own, or the one for a server elsewhere
# (also before "connect", so that a desktop can be imported first).
room_compose() {
  if [ -f .env ]; then docker compose --profile room-default "$@"; else node "$@"; fi
}
room_service() { if [ -f .env ]; then echo room-default; else echo room; fi; }
room_home() { volume room_compose "$(if [ -f .env ]; then echo room-default-home; else echo home; fi)"; }
room_running() { room_compose ps -q --status running "$(room_service)"; }

# Runs a command as root in a container of the room's image that has the
# room's home folder and nothing running in it. MSYS_NO_PATHCONV: Git Bash
# on Windows would rewrite the paths meant for the container.
room_run() {
  MSYS_NO_PATHCONV=1 room_compose run --rm --no-deps -T --user 0 --entrypoint "$1" "$(room_service)" "${@:2}"
}

export_room() {
  local file=${1:-cozycast-room.tar.gz} home service running ok=1
  home=$(room_home)
  [ -n "$home" ] || die "This checkout has no room desktop yet."
  [ ! -e "$file" ] || die "$file exists already: move it away, or name another file."
  service=$(room_service)
  running=$(room_running)
  if [ -n "$running" ]; then
    # A copy of a running Firefox's profile is not one it can count on.
    sure "The room is stopped while its files are copied; whoever is in it sees it offline. Go on?" || die "Nothing exported."
    room_compose stop "$service"
  fi
  # Caches stay behind: large, and they fill again by themselves.
  (umask 077 && room_run tar -czf - --warning=no-file-ignored --exclude=./.cache -C /home/cozycast . > "$file.part") || ok=""
  [ -z "$running" ] || room_compose start "$service"
  if [ -z "$ok" ]; then
    rm -f "$file.part"
    die "The export failed; nothing was written."
  fi
  mv "$file.part" "$file"
  echo
  echo "Wrote $file ($(du -h "$file" | cut -f1)). It holds the room browser's logins:"
  echo "copy it over securely (scp), then on the other computer:"
  echo "  $0 import-room $(basename "$file")"
  echo "and delete the file on both once the room runs there."
}

import_room() {
  [ $# -eq 1 ] || die "Which file? For example: $0 import-room cozycast-room.tar.gz"
  local file=$1 first home service running
  [ -f "$file" ] || die "$file was not found."
  # Read all of it before anything is deleted: a file cut off on the way
  # must not replace a desktop by half of one.
  first=$(tar -tzf "$file" | sed -n 1p) || die "$file is damaged or cut off: copy it over again."
  [ "$first" = ./ ] || die "$file is not a room export (\"$0 export-room\" makes one)."
  [ -f .env ] || [ -f .env.node ] || echo "Not a server (no .env): importing for a room that runs for a server elsewhere."
  home=$(room_home)
  if [ -n "$home" ]; then
    sure "This replaces everything in this room's home folder (its files and Firefox profile)." || die "Nothing imported."
  fi
  service=$(room_service)
  running=$(room_running)
  [ -z "$running" ] || room_compose stop "$service"
  room_run sh -c 'find /home/cozycast -mindepth 1 -delete && tar -xzf - -C /home/cozycast' < "$file" ||
    die "The import failed and the room's home folder is incomplete: free some space and import again."
  echo
  echo "The room's home folder is now the one from $file."
  if [ -n "$running" ]; then
    room_compose start "$service"
  elif [ ! -f .env ] && [ ! -f .env.node ]; then
    echo "Next: $0 connect <server>"
  else
    echo "Next: $0 start"
  fi
}

# The server's own compose project, for volume().
server_compose() { docker compose "$@"; }

# Runs the server program once on the server's data, beside a server that
# may be running. Its image has no shell: exporting and importing are
# commands of the program itself.
server_run() { docker compose run --rm --no-deps -T server "$@"; }

export_server() {
  [ -f .env ] || die "This checkout is not a server (no .env)."
  local file=${1:-cozycast-server.tar.gz}
  [ ! -e "$file" ] || die "$file exists already: move it away, or name another file."
  [ -n "$(volume server_compose server-data)" ] || die "This server has no data yet."
  # The server goes on running: its database is copied as of one moment.
  if ! (umask 077 && server_run export > "$file.part"); then
    rm -f "$file.part"
    die "The export failed; nothing was written. (A server from before this command cannot export: \"$0 update\" first.)"
  fi
  mv "$file.part" "$file"
  echo
  echo "Wrote $file ($(du -h "$file" | cut -f1)): the accounts with their password hashes, chat,"
  echo "room settings, pairings and this server's keys. Copy it over securely (scp),"
  echo "then on the other server:"
  echo "  $0 import-server $(basename "$file")"
  echo "and delete the file on both once the new server runs. A room's desktop"
  echo "is not in it: \"$0 export-room\" moves that."
}

import_server() {
  [ $# -eq 1 ] || die "Which file? For example: $0 import-server cozycast-server.tar.gz"
  local file=$1 first running
  [ -f .env ] || die "Set this server up first (\"$0 setup\"): its .env stays its own."
  [ -f "$file" ] || die "$file was not found."
  first=$(tar -tzf "$file" | sed -n 1p) || die "$file is damaged or cut off: copy it over again."
  [ "$first" = cozycast-server.json ] || die "$file is not a server export (\"$0 export-server\" makes one)."
  if [ -n "$(volume server_compose server-data)" ]; then
    sure "This replaces everything this server knows: its accounts, chat, room settings and pairings." || die "Nothing imported."
  fi
  running=$(docker compose ps -q --status running server)
  [ -z "$running" ] || docker compose stop server
  if ! server_run import < "$file"; then
    [ -z "$running" ] || docker compose start server
    die "The import failed. An export that is refused leaves the server's data as it was."
  fi
  echo "Log in with the accounts of the old server: ADMIN_PASSWORD in .env is only"
  echo "for an \"admin\" account that does not exist yet."
  if [ -n "$running" ]; then
    docker compose start server
  else
    echo "Next: $0 start"
  fi
}

case "${1:-}" in
  setup | start | stop | status | update | forget) [ $# -eq 1 ] || die "$1 takes no arguments."; "$1" ;;
  logs) shift; logs "$@" ;;
  connect) shift; connect "$@" ;;
  export-room) [ $# -le 2 ] || die "export-room takes one file name."; shift; export_room "$@" ;;
  import-room) shift; import_room "$@" ;;
  export-server) [ $# -le 2 ] || die "export-server takes one file name."; shift; export_server "$@" ;;
  import-server) shift; import_server "$@" ;;
  "" | help | -h | --help) usage ;;
  *) usage >&2; exit 1 ;;
esac
