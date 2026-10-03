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

    # Only the room parts of the archive; never restore owners or absolute paths.
    if ! tar -xzf "$archive" -C "$tmp" --no-same-owner --no-same-permissions \
        --wildcards 'home/*' 'firefox-profile/*' 2>/dev/null; then
        echo "import-home: archive has no room desktop data"
    fi

    files=0
    if [ -d "$tmp/home" ]; then
        # Existing files with the same name are kept (e.g. after a manual copy).
        cp -Rn "$tmp/home/." "$home/"
        files=$(find "$tmp/home" -type f | wc -l)
    fi
    if [ -d "$tmp/firefox-profile" ]; then
        rm -rf "$profile"
        mkdir -p "$profile"
        cp -R "$tmp/firefox-profile/." "$profile/"
        # A profile from another machine must not look locked.
        rm -f "$profile/lock" "$profile/.parentlock"
        # Exports copy the profile while the old Firefox runs, so there is
        # only the crash-recovery copy of the open tabs. Make it the regular
        # session file, which is what Firefox restores on a normal start.
        if [ ! -f "$profile/sessionstore.jsonlz4" ] && [ -f "$profile/sessionstore-backups/recovery.jsonlz4" ]; then
            cp "$profile/sessionstore-backups/recovery.jsonlz4" "$profile/sessionstore.jsonlz4"
            rm -f "$profile/sessionCheckpoints.json"
        fi
        echo "import-home: imported the Firefox profile"
    fi
    chown -R neko:neko "$home"
    echo "import-home: imported $files files into $home"
}

if [ -n "$archive" ] && [ -f "$archive" ] && [ ! -e "$marker" ]; then
    if import_home; then
        date -u > "$marker"
        chown neko:neko "$marker"
    else
        echo "import-home: import failed; starting without it" >&2
    fi
fi
