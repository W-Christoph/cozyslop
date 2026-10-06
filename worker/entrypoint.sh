#!/bin/sh
# Container entrypoint: prepare the room, then hand over to neko's normal
# startup (supervisord).
#
# 1. Stream pipelines: every combination of a bitrate ladder, a set of
#    stream sizes and a set of x264 presets becomes a neko capture pipeline
#    with the id b<kbps>-s<percent>-<preset> (e.g. b2500-s100-veryfast).
#    Rooms pick one in their settings; neko only runs the pipelines someone
#    is watching, so offering many costs nothing. Override with
#    COZYCAST_STREAM_BITRATES (kbit/s), COZYCAST_STREAM_SCALES (percent of
#    the desktop size), COZYCAST_X264_PRESETS (fastest first) and
#    COZYCAST_X264_PRESET (the default), or set NEKO_CAPTURE_VIDEO_PIPELINES
#    yourself to skip this.
# 2. The room's neko admin token: supplied as COZYCAST_NEKO_TOKEN (directly,
#    or in COZYCAST_ENV_FILE written by the agent of a paired room), or
#    derived from COZYCAST_NEKO_SECRET and the room's name (COZYCAST_ROOM),
#    the same way the server does it
#    (config.NekoToken). neko runs as the desktop's user, so whoever holds
#    the remote can read neko's token; this way it is the token of this
#    room only, and the secret itself never reaches the desktop.
# 3. A one-time import of the old CozyCast room desktop (import-home.sh).
# 4. The desktop's user was "neko" once, with /home/neko as its home. A
#    desktop from then has that path in some of its settings (the panel's
#    folder menu, the places of the desktop icons); they are pointed at the
#    home folder's place now.
set -eu

stream_pipelines() {
    bitrates=${COZYCAST_STREAM_BITRATES:-1000 1500 2500 4000 6000 8000}
    scales=${COZYCAST_STREAM_SCALES:-100 75 67 50}
    preset=${COZYCAST_X264_PRESET:-veryfast}
    presets=${COZYCAST_X264_PRESETS:-ultrafast superfast veryfast}
    # The default preset is always offered.
    case " $presets " in *" $preset "*) ;; *) presets="$presets $preset" ;; esac
    preferred=${COZYCAST_STREAM_DEFAULT:-b2500-s100-$preset}

    ids="" json="" first="" found=""
    for b in $bitrates; do
        for s in $scales; do
            size=""
            if [ "$s" != 100 ]; then
                # Even dimensions: x264 needs them for 4:2:0 video.
                size="\"width\": \"round(width * $s / 200) * 2\", \"height\": \"round(height * $s / 200) * 2\", "
            fi
            for p in $presets; do
                id="b$b-s$s-$p"
                [ -n "$first" ] || first=$id
                [ "$id" != "$preferred" ] || found=1
                json="$json${json:+, }\"$id\": {\"fps\": \"fps\", $size\"gst_prefix\": \"! video/x-raw,format=I420\", \
\"gst_encoder\": \"x264enc\", \"gst_params\": {\"threads\": 2, \"bitrate\": $b, \"key-int-max\": \"fps * 2\", \
\"byte-stream\": true, \"tune\": \"zerolatency\", \"speed-preset\": \"$p\"}, \
\"gst_suffix\": \"! video/x-h264,stream-format=byte-stream,profile=constrained-baseline\"}"
                ids="$ids${ids:+ }$id"
            done
        done
    done

    # neko's default stream is the first id; it must be one that exists.
    default=$first
    [ -z "$found" ] || default=$preferred
    ids="$default $(printf '%s\n' $ids | grep -vx "$default" | tr '\n' ' ')"

    NEKO_CAPTURE_VIDEO_CODEC=h264
    NEKO_CAPTURE_VIDEO_PIPELINES="{$json}"
    # Space separated: neko does not split this list on commas.
    NEKO_CAPTURE_VIDEO_IDS=${ids% }
    export NEKO_CAPTURE_VIDEO_CODEC NEKO_CAPTURE_VIDEO_PIPELINES NEKO_CAPTURE_VIDEO_IDS
    echo "entrypoint: $(printf '%s\n' $ids | wc -l) stream pipelines, default $default"
}

if [ -z "${NEKO_CAPTURE_VIDEO_PIPELINES:-}" ]; then
    stream_pipelines
fi

# A room on another computer (compose.node.yaml) gets its name and token
# from the agent, which fetches them from the server once paired.
if [ -n "${COZYCAST_ENV_FILE:-}" ]; then
    while [ ! -s "$COZYCAST_ENV_FILE" ]; do
        echo "entrypoint: waiting for the agent to pair with the server"
        sleep 5
    done
    set -a
    . "$COZYCAST_ENV_FILE"
    set +a
fi

if [ -n "${COZYCAST_NEKO_TOKEN:-}" ]; then
    NEKO_SESSION_API_TOKEN=$COZYCAST_NEKO_TOKEN
    export NEKO_SESSION_API_TOKEN
elif [ -n "${COZYCAST_NEKO_SECRET:-}" ]; then
    : "${COZYCAST_ROOM:?set COZYCAST_ROOM to the room's name in COZYCAST_ROOMS}"
    sum=$(printf 'cozycast-neko-token:%s:%s' "$COZYCAST_ROOM" "$COZYCAST_NEKO_SECRET" | sha256sum)
    NEKO_SESSION_API_TOKEN=${sum%% *}
    export NEKO_SESSION_API_TOKEN
    # Everything started from here on inherits the environment.
    unset sum
fi

# Neither credential source reaches the desktop processes.
unset COZYCAST_NEKO_SECRET COZYCAST_NEKO_TOKEN

/usr/local/bin/cozycast-import-home

home=/home/$USER
if [ -d "$home/.config" ]; then
    grep -rIl '/home/neko' "$home/.config" 2>/dev/null | while IFS= read -r file; do
        # sed -i writes a new file: give it back to the desktop's user.
        sed -i "s|/home/neko|$home|g" "$file"
        chown "$USER:$USER" "$file"
    done
fi

exec "$@"
