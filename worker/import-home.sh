#!/bin/sh
# Called by entrypoint.sh before the desktop starts: import the old CozyCast
# room's home folder (files, folders, Firefox profile) from a migration
# archive, once.
#
# COZYCAST_IMPORT_HOME points at cozycast-export.tar.gz (see
# migrate/export-cozycast.sh). A marker in the home folder makes sure an
# import never runs twice, so the archive can stay in place.
set -eu

archive=${COZYCAST_IMPORT_HOME:-}
home=/home/neko
marker="$home/.cozycast-imported"
profile="$home/.mozilla/firefox/profile.default"

import_home() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    # Read the whole archive before touching the home folder: a damaged or
    # cut-off archive must not import half a desktop.
    tar -tzf "$archive" > "$tmp/list"
    parts=""
    for part in home firefox-profile; do
        if grep -q "^$part/" "$tmp/list"; then
            parts="$parts $part"
        fi
    done
    if [ -z "$parts" ]; then
        # No marker for this: an accounts-only archive may be followed by
        # one with the desktop.
        echo "import-home: archive has no room desktop data"
        return 3
    fi
    # Only the room parts of the archive; never restore owners or absolute paths.
    mkdir "$tmp/data"
    # shellcheck disable=SC2086 # $parts is a list
    tar -xzf "$archive" -C "$tmp/data" --no-same-owner --no-same-permissions $parts

    files=0
    if [ -d "$tmp/data/home" ]; then
        # Existing files with the same name are kept (e.g. after a manual copy).
        cp -Rn "$tmp/data/home/." "$home/"
        files=$(find "$tmp/data/home" -type f | wc -l)
    fi
    if [ -d "$tmp/data/firefox-profile" ]; then
        # Build the new profile next to the current one and swap them at the
        # end, so a failed copy leaves the current profile as it was.
        new="$profile.import"
        rm -rf "$new"
        mkdir -p "${profile%/*}"
        cp -R "$tmp/data/firefox-profile" "$new"
        # A profile from another machine must not look locked.
        rm -f "$new/lock" "$new/.parentlock"
        # Exports copy the profile while the old Firefox runs, so there is
        # only the crash-recovery copy of the open tabs. Make it the regular
        # session file, which is what Firefox restores on a normal start.
        if [ ! -f "$new/sessionstore.jsonlz4" ] && [ -f "$new/sessionstore-backups/recovery.jsonlz4" ]; then
            cp "$new/sessionstore-backups/recovery.jsonlz4" "$new/sessionstore.jsonlz4"
            rm -f "$new/sessionCheckpoints.json"
        fi
        rm -rf "$profile"
        mv "$new" "$profile"
        echo "import-home: imported the Firefox profile"
    fi
    chown -R neko:neko "$home"
    echo "import-home: imported $files files into $home"
}

if [ -n "$archive" ] && [ -f "$archive" ] && [ ! -e "$marker" ]; then
    # Run the import on its own, not as an `if` condition: there, set -e is
    # off and a failing step would not stop it.
    set +e
    (set -e; import_home)
    status=$?
    set -e
    case $status in
        0)
            date -u > "$marker"
            chown neko:neko "$marker"
            ;;
        3) ;;
        *) echo "import-home: import failed; starting without it (it is tried again on the next start)" >&2 ;;
    esac
fi
