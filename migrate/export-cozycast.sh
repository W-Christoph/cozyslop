#!/usr/bin/env bash
set -euo pipefail
umask 077

usage() {
    cat >&2 <<EOF
Usage: $0 [--check | --accounts-only | --desktop-only] [path to old cozycast checkout]
  --check          report the space a full export needs and what is free, then stop
  --accounts-only  export accounts and avatars, without the room desktop
  --desktop-only   export the room desktop alone, for a server that already has
                   the accounts; the old CozyCast does not have to be running
EOF
    exit 1
}
mode=all
case ${1:-} in
    --check | --accounts-only | --desktop-only)
        mode=${1#--}
        shift
        ;;
    -*) usage ;;
esac
if [ "$#" -gt 1 ]; then
    usage
fi
checkout=${1:-.}

avatar_dir="$checkout/data/cozycast-server/avatar"
# The old room desktop's home folder (shared by all workers): the user's
# files and folders, and the Firefox profile (logins, bookmarks, open tabs).
# Hidden config folders stay behind: they belong to the old desktop setup
# and reference the old user. So do caches and worker runtime files.
home="$checkout/data/cozycast-worker/cozycast"
home_entries=(-mindepth 1 -maxdepth 1 ! -name '.*' ! -name '*.pid' ! -name '*.log' ! -name 'worker.restart')
profile_excludes=(--exclude=cache2 --exclude=startupCache --exclude=thumbnails --exclude=crashes
    --exclude=minidumps --exclude=datareporting --exclude=saved-telemetry-pings
    --exclude=lock --exclude=.parentlock --exclude='*.lock')
if [ "$mode" = desktop-only ] && [ ! -d "$home" ]; then
    echo "Room desktop home folder missing: $home" >&2
    exit 1
fi

# The default Firefox profile: the install default if there is one,
# otherwise the profile marked Default=1, otherwise the first one.
profile_name=""
ini="$home/.mozilla/firefox/profiles.ini"
if [ "$mode" != accounts-only ] && [ -f "$ini" ]; then
    profile_name=$(awk -F= '
        /^\[/ { install = ($0 ~ /^\[Install/); path = ""; next }
        install && $1 == "Default" { print $2; found = 1; exit }
        $1 == "Path" { path = $2; if (first == "") first = $2 }
        $1 == "Default" && $2 == "1" && path != "" { marked = path }
        END { if (!found) print (marked != "" ? marked : first) }' "$ini")
    if [ -z "$profile_name" ] || [ ! -d "$home/.mozilla/firefox/$profile_name" ]; then
        profile_name=""
    fi
fi

# Sizes are in KiB.
total() { awk '{ kb += $1 } END { print kb + 0 }'; }
human() { awk -v kb="$1" 'BEGIN { if (kb >= 1048576) printf "%.1f GB", kb / 1048576; else printf "%d MB", (kb + 1023) / 1024 }'; }
free() { df -Pk "$1" | awk 'NR == 2 { print $4 }'; }

avatar_kb=0
desktop_kb=0
if [ "$mode" != desktop-only ] && [ -d "$avatar_dir" ]; then
    avatar_kb=$(du -sk "$avatar_dir" | total)
fi
if [ "$mode" != accounts-only ] && [ -d "$home" ]; then
    desktop_kb=$(find "$home" "${home_entries[@]}" -exec du -sk {} + | total)
    if [ -n "$profile_name" ]; then
        profile_kb=$(du -sk "${profile_excludes[@]}" "$home/.mozilla/firefox/$profile_name" | total)
        desktop_kb=$((desktop_kb + profile_kb))
    fi
fi

# The export is put together in the temporary directory: a copy of the files,
# then the archive next to it. Videos and images hardly compress, so the
# archive is assumed to get as large as the copy. The reserve keeps the disk
# from filling up under a running CozyCast.
tmpdir=${TMPDIR:-/tmp}
reserve_kb=102400
export_kb=$((avatar_kb + desktop_kb))
tmp_need_kb=$((2 * export_kb + reserve_kb))
tmp_free_kb=$(free "$tmpdir")
needs="up to $(human "$tmp_need_kb") in $tmpdir ($(human "$tmp_free_kb") free)"
short=""
[ "$tmp_free_kb" -ge "$tmp_need_kb" ] || short=1
# Moving the archive to another filesystem copies it.
if [ "$(stat -c %d "$tmpdir")" != "$(stat -c %d .)" ]; then
    out_need_kb=$((export_kb + reserve_kb))
    out_free_kb=$(free .)
    needs="$needs and up to $(human "$out_need_kb") in $PWD ($(human "$out_free_kb") free)"
    [ "$out_free_kb" -ge "$out_need_kb" ] || short=1
fi
if [ "$mode" = check ]; then
    echo "Avatars: $(human "$avatar_kb"). Room desktop: $(human "$desktop_kb")."
    echo "A full export needs $needs."
    echo "Importing the room desktop needs up to $(human $((2 * desktop_kb))) in the new server's Docker storage; $(human "$desktop_kb") of it stays in use."
fi
if [ -n "$short" ]; then
    hint="Set TMPDIR to a directory with more room"
    if [ "$mode" = all ] || [ "$mode" = check ]; then
        hint="$hint, or export with --accounts-only (the room desktop can follow with --desktop-only)"
    fi
    if [ "$mode" = check ]; then
        echo "Not enough free space for a full export." >&2
    else
        echo "Not enough free space: the export needs $needs." >&2
    fi
    echo "$hint." >&2
    exit 1
fi
if [ "$mode" = check ]; then
    echo "Enough space for a full export."
    exit 0
fi

container=""
if [ "$mode" != desktop-only ]; then
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
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

counts=""
avatars=0
if [ "$mode" != desktop-only ]; then
    # Read all columns so extra fields and optional stream settings need no
    # schema-specific handling. Both output lines come from one SQL snapshot.
    # Logins are the exception: browsers logged in to the old server hold a
    # refresh token, and the new server lets each start a session once. Only
    # a hash of the token is exported, so the archive cannot be used to log in.
    sql="WITH export AS (
        SELECT json_build_object(
            'format', 'cozycast-export', 'version', 1, 'exportedAt', now(),
            'users', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM users t),
            'room_persistence', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_persistence t),
            'room_permission', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_permission t),
            'room_invite', (SELECT coalesce(json_agg(row_to_json(t)), '[]'::json) FROM room_invite t),
            'refresh_token', (SELECT coalesce(json_agg(json_build_object('username', username,
                'token_sha256', encode(sha256(convert_to(refresh_token, 'UTF8')), 'hex'))), '[]'::json)
                FROM refresh_token WHERE NOT token_revoked)
        ) AS payload
    ), lines AS (
        SELECT 1 AS n, payload::text AS line FROM export
        UNION ALL
        SELECT 2, format('%s users, %s rooms, %s permissions, %s invites, %s logins',
            json_array_length(payload->'users'), json_array_length(payload->'room_persistence'),
            json_array_length(payload->'room_permission'), json_array_length(payload->'room_invite'),
            json_array_length(payload->'refresh_token')) FROM export
    ) SELECT line FROM lines ORDER BY n;"
    docker exec "$container" psql -U cozycast -d cozycast -X -v ON_ERROR_STOP=1 -At -c "$sql" > "$tmp/output"
    sed '$d' "$tmp/output" > "$tmp/export.json"
    counts=$(tail -n 1 "$tmp/output")
    rm "$tmp/output"

    if [ -d "$avatar_dir" ]; then
        cp -R "$avatar_dir" "$tmp/avatar"
        avatars=$(find "$tmp/avatar" -type f | wc -l | tr -d ' ')
    else
        echo "Avatar directory missing; exporting database only." >&2
    fi
fi

home_files=0
if [ "$mode" != accounts-only ]; then
    if [ -d "$home" ]; then
        mkdir "$tmp/home"
        # "{} +", not "{} \;": only then does a failed copy fail find, and the export.
        find "$home" "${home_entries[@]}" -exec cp -R -t "$tmp/home/" {} +
        home_files=$(find "$tmp/home" -type f | wc -l | tr -d ' ')
        if [ -n "$profile_name" ]; then
            mkdir "$tmp/firefox-profile"
            tar -C "$home/.mozilla/firefox/$profile_name" "${profile_excludes[@]}" -cf - . |
                tar -C "$tmp/firefox-profile" -xf -
        fi
    else
        echo "Room desktop home folder missing; exporting accounts only." >&2
    fi
fi

# Create privately before replacing the destination (even if it exists with
# broader permissions), and keep partial archives out of the working directory.
parts=()
for part in export.json avatar home firefox-profile; do
    if [ -e "$tmp/$part" ]; then
        parts+=("$part")
    fi
done
tar -czf "$tmp/cozycast-export.tar.gz" -C "$tmp" "${parts[@]}"
chmod 600 "$tmp/cozycast-export.tar.gz"
mv -f "$tmp/cozycast-export.tar.gz" ./cozycast-export.tar.gz
if [ "$mode" = desktop-only ]; then
    echo "Exported the room desktop to cozycast-export.tar.gz, without accounts."
else
    echo "Exported $counts, $avatars avatar files to cozycast-export.tar.gz."
fi
if [ "$mode" = accounts-only ]; then
    echo "Room desktop: not included; export it later with --desktop-only."
else
    echo "Room desktop: $home_files files from the home folder${profile_name:+, Firefox profile $profile_name}."
fi
echo "Archive size: $(du -h cozycast-export.tar.gz | cut -f1)."
if [ "$mode" = desktop-only ]; then
    echo "Import it on a server that already has the accounts. It contains the room browser's logins: transfer it securely and delete it from both machines after importing."
else
    echo "This file contains password hashes. Transfer it securely and delete it from both machines after importing."
fi
