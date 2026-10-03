#!/usr/bin/env bash
set -euo pipefail
umask 077

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [path to old cozycast checkout]" >&2
    exit 1
fi
checkout=${1:-.}
running=$(docker ps --format '{{.Names}}')
container=${POSTGRES_CONTAINER:-}
if [ -z "$container" ]; then
    for candidate in cozycast_postgres_1 cozycast-postgres-1; do
        if printf '%s\n' "$running" | grep -Fxq "$candidate"; then
            container=$candidate
            break
        fi
    done
fi
if [ -z "$container" ]; then
    matches=$(printf '%s\n' "$running" | awk '/postgres/ && /cozycast/')
    count=$(printf '%s\n' "$matches" | awk 'NF { n++ } END { print n+0 }')
    if [ "$count" -ne 1 ]; then
        echo "Cannot identify one running CozyCast Postgres container. Set POSTGRES_CONTAINER to its name." >&2
        exit 1
    fi
    container=$matches
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Read all columns so extra fields and optional stream settings need no
# schema-specific handling. Both output lines come from one SQL snapshot.
sql="WITH export AS (
    SELECT json_build_object(
        'format', 'cozycast-export', 'version', 1, 'exportedAt', now(),
        'users', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM users t),
        'room_persistence', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_persistence t),
        'room_permission', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_permission t),
        'room_invite', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_invite t)
    ) AS payload
), lines AS (
    SELECT 1 AS n, payload::text AS line FROM export
    UNION ALL
    SELECT 2, format('%s users, %s rooms, %s permissions, %s invites',
        json_array_length(payload->'users'), json_array_length(payload->'room_persistence'),
        json_array_length(payload->'room_permission'), json_array_length(payload->'room_invite')) FROM export
) SELECT line FROM lines ORDER BY n;"
docker exec "$container" psql -U cozycast -d cozycast -X -v ON_ERROR_STOP=1 -At -c "$sql" > "$tmp/output"
sed '$d' "$tmp/output" > "$tmp/export.json"
counts=$(tail -n 1 "$tmp/output")
rm "$tmp/output"

avatars=0
if [ -d "$checkout/data/cozycast-server/avatar" ]; then
    cp -R "$checkout/data/cozycast-server/avatar" "$tmp/avatar"
    avatars=$(find "$tmp/avatar" -type f | wc -l | tr -d ' ')
else
    echo "Avatar directory missing; exporting database only." >&2
fi
# The old room desktop's home folder (shared by all workers): the user's
# files and folders, and the Firefox profile (logins, bookmarks, open tabs).
# Hidden config folders stay behind: they belong to the old desktop setup
# and reference the old user. So do caches and worker runtime files.
home="$checkout/data/cozycast-worker/cozycast"
home_files=0
profile_name=""
if [ -d "$home" ]; then
    mkdir "$tmp/home"
    find "$home" -mindepth 1 -maxdepth 1 ! -name '.*' ! -name '*.pid' ! -name '*.log' ! -name 'worker.restart' \
        -exec cp -R {} "$tmp/home/" \;
    home_files=$(find "$tmp/home" -type f | wc -l | tr -d ' ')

    # The default Firefox profile: the install default if there is one,
    # otherwise the profile marked Default=1, otherwise the first one.
    ini="$home/.mozilla/firefox/profiles.ini"
    if [ -f "$ini" ]; then
        profile_name=$(awk -F= '
            /^\[/ { install = ($0 ~ /^\[Install/); path = ""; next }
            install && $1 == "Default" { print $2; found = 1; exit }
            $1 == "Path" { path = $2; if (first == "") first = $2 }
            $1 == "Default" && $2 == "1" && path != "" { marked = path }
            END { if (!found) print (marked != "" ? marked : first) }' "$ini")
    fi
    if [ -n "$profile_name" ] && [ -d "$home/.mozilla/firefox/$profile_name" ]; then
        mkdir "$tmp/firefox-profile"
        tar -C "$home/.mozilla/firefox/$profile_name" \
            --exclude=cache2 --exclude=startupCache --exclude=thumbnails --exclude=crashes \
            --exclude=minidumps --exclude=datareporting --exclude=saved-telemetry-pings \
            --exclude=lock --exclude=.parentlock --exclude='*.lock' \
            -cf - . | tar -C "$tmp/firefox-profile" -xf -
    else
        profile_name=""
    fi
else
    echo "Room desktop home folder missing; exporting accounts only." >&2
fi

# Create privately before replacing the destination (even if it exists with
# broader permissions), and keep partial archives out of the working directory.
parts=(export.json)
for part in avatar home firefox-profile; do
    if [ -d "$tmp/$part" ]; then
        parts+=("$part")
    fi
done
tar -czf "$tmp/cozycast-export.tar.gz" -C "$tmp" "${parts[@]}"
chmod 600 "$tmp/cozycast-export.tar.gz"
mv -f "$tmp/cozycast-export.tar.gz" ./cozycast-export.tar.gz
echo "Exported $counts, $avatars avatar files to cozycast-export.tar.gz."
echo "Room desktop: $home_files files from the home folder${profile_name:+, Firefox profile $profile_name}."
echo "Archive size: $(du -h cozycast-export.tar.gz | cut -f1)."
echo "This file contains password hashes. Transfer it securely and delete it from both machines after importing."
