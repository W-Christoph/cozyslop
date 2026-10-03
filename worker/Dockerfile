# CozyCast room worker: an unmodified neko XFCE desktop plus Firefox and VLC.
# Capture, encoding, WebRTC and input are handled by neko itself; this image
# only adds the apps we want on the desktop.
ARG NEKO_VERSION=3.1.6

FROM ghcr.io/m1k1o/neko/firefox:${NEKO_VERSION} AS firefox

FROM ghcr.io/m1k1o/neko/xfce:${NEKO_VERSION}

RUN set -eux; \
    sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources; \
    apt-get update; \
    apt-get install -y --no-install-recommends vlc libdbus-glib-1-2; \
    apt-get clean -y; \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/*

# Reuse neko's Firefox build instead of downloading it again. Both images share
# the same base, so the runtime libraries already match.
COPY --from=firefox /usr/lib/firefox /usr/lib/firefox
COPY --from=firefox --chown=neko:neko /home/neko/.mozilla /home/neko/.mozilla
RUN ln -s /usr/lib/firefox/firefox /usr/bin/firefox

COPY firefox/mozilla.cfg /usr/lib/firefox/mozilla.cfg
COPY firefox/policies.json /usr/lib/firefox/distribution/policies.json
COPY supervisord.conf /etc/neko/supervisord/cozycast.conf
