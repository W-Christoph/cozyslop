#!/bin/sh
# Container entrypoint: prepare the room, then hand over to neko's normal
# startup (supervisord).
#
# 1. Stream pipelines: every combination of a bitrate ladder and a set of
#    stream sizes becomes a neko capture pipeline with the id
#    b<kbps>-s<percent> (e.g. b2500-s100). Rooms pick one in their settings;
#    neko only runs the pipelines someone is watching, so offering many
#    costs nothing. Override with COZYCAST_STREAM_BITRATES (kbit/s) and
#    COZYCAST_STREAM_SCALES (percent of the desktop size), or set
#    NEKO_CAPTURE_VIDEO_PIPELINES yourself to skip this.
# 2. A one-time import of the old CozyCast room desktop (import-home.sh).
set -eu

stream_pipelines() {
    bitrates=${COZYCAST_STREAM_BITRATES:-1000 1500 2500 4000 6000 8000}
    scales=${COZYCAST_STREAM_SCALES:-100 75 67 50}
    preferred=${COZYCAST_STREAM_DEFAULT:-b2500-s100}
    preset=${COZYCAST_X264_PRESET:-veryfast}

    ids="" json="" first="" found=""
    for b in $bitrates; do
        for s in $scales; do
            id="b$b-s$s"
            [ -n "$first" ] || first=$id
            [ "$id" != "$preferred" ] || found=1
            size=""
            if [ "$s" != 100 ]; then
                # Even dimensions: x264 needs them for 4:2:0 video.
                size="\"width\": \"round(width * $s / 200) * 2\", \"height\": \"round(height * $s / 200) * 2\", "
            fi
            json="$json${json:+, }\"$id\": {\"fps\": \"fps\", $size\"gst_prefix\": \"! video/x-raw,format=I420\", \
\"gst_encoder\": \"x264enc\", \"gst_params\": {\"threads\": 2, \"bitrate\": $b, \"key-int-max\": \"fps * 2\", \
\"byte-stream\": true, \"tune\": \"zerolatency\", \"speed-preset\": \"$preset\"}, \
\"gst_suffix\": \"! video/x-h264,stream-format=byte-stream,profile=constrained-baseline\"}"
            ids="$ids${ids:+ }$id"
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

/usr/local/bin/cozycast-import-home

exec "$@"
