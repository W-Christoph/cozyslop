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
# Create privately before replacing the destination (even if it exists with
# broader permissions), and keep partial archives out of the working directory.
if [ -d "$tmp/avatar" ]; then
    tar -czf "$tmp/cozycast-export.tar.gz" -C "$tmp" export.json avatar
else
    tar -czf "$tmp/cozycast-export.tar.gz" -C "$tmp" export.json
fi
chmod 600 "$tmp/cozycast-export.tar.gz"
mv -f "$tmp/cozycast-export.tar.gz" ./cozycast-export.tar.gz
echo "Exported $counts, $avatars avatar files to cozycast-export.tar.gz."
echo "This file contains password hashes. Transfer it securely and delete it from both machines after importing."
