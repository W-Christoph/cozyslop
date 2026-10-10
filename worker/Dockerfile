# CozyCast room worker: an unmodified neko XFCE desktop plus Firefox, VLC and
# the Ristretto image viewer.
# Capture, encoding, WebRTC and input are handled by neko itself; this image
# only adds the apps we want on the desktop.
ARG NEKO_VERSION=3.1.6

FROM ghcr.io/m1k1o/neko/firefox:${NEKO_VERSION} AS firefox

FROM ghcr.io/m1k1o/neko/xfce:${NEKO_VERSION}

# The desktop's user is "cozycast", as in the old CozyCast, not neko's
# "neko": the name shows in the panel, the terminal prompt and the file
# manager. neko's supervisord configuration runs everything as USER, with
# /home/USER as its home. It also gives its control socket to a group named
# "neko", so a group of that name stays.
ENV USER=cozycast
RUN usermod -l cozycast -d /home/cozycast -m neko && \
    groupmod -n cozycast neko && \
    groupadd neko && usermod -aG neko cozycast

RUN set -eux; \
    sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources; \
    apt-get update; \
    # librsvg2-common lets GTK draw SVG icons. The desktop's icon theme is
    # SVG apart from a 16 pixel set; without it, file and toolbar icons are
    # those 16 pixels blown up.
    apt-get install -y --no-install-recommends vlc ristretto tumbler libdbus-glib-1-2 librsvg2-common greybird-gtk-theme; \
    apt-get clean -y; \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/*

# A dark desktop, to match CozyCast and the room's Firefox: the Greybird-dark
# theme for windows and their frames, with the installed Adwaita icons.
# (XFCE's own defaults name a theme and an icon theme that are not installed,
# Xfce and Tango.) Someone who picks another look in the desktop's settings
# keeps it; a desktop that existed before keeps its window frames until
# "Window Manager > Style" is changed there.
RUN sed -i \
    -e 's|\(name="ThemeName" type="string" value="\)[^"]*|\1Greybird-dark|' \
    -e 's|\(name="IconThemeName" type="string" value="\)[^"]*|\1Adwaita|' \
    /etc/xdg/xfce4/xfconf/xfce-perchannel-xml/xsettings.xml && \
    grep -q 'value="Greybird-dark"' /etc/xdg/xfce4/xfconf/xfce-perchannel-xml/xsettings.xml
COPY --chmod=0644 xfce/xfwm4.xml /etc/xdg/xfce4/xfconf/xfce-perchannel-xml/xfwm4.xml

# Reuse neko's Firefox build instead of downloading it again. Both images share
# the same base, so the runtime libraries already match.
COPY --from=firefox /usr/lib/firefox /usr/lib/firefox
COPY --from=firefox --chown=cozycast:cozycast /home/neko/.mozilla /home/cozycast/.mozilla
# Downloads is a volume; creating it here gives fresh volumes the right owner.
RUN ln -s /usr/lib/firefox/firefox /usr/bin/firefox && \
    install -d -o cozycast -g cozycast /home/cozycast/Downloads

# Explicit modes: Firefox runs as neko and must be able to read these,
# whatever permissions the files have in the checkout.
COPY --chmod=0644 firefox/mozilla.cfg /usr/lib/firefox/mozilla.cfg
COPY --chmod=0644 firefox/policies.json /usr/lib/firefox/distribution/policies.json
COPY --chmod=0644 supervisord.conf /etc/neko/supervisord/cozycast.conf

# Generates the stream pipelines and imports an old CozyCast room's home
# folder on first start, then runs neko.
COPY --chmod=0755 import-home.sh /usr/local/bin/cozycast-import-home
COPY --chmod=0755 entrypoint.sh /usr/local/bin/cozycast-entrypoint
# Reports the title of the window in front to the CozyCast server.
COPY --chmod=0755 window-title.py /usr/local/bin/cozycast-window-title
# Plays a file of the Downloads folder in VLC when the CozyCast server asks.
COPY --chmod=0755 play.py /usr/local/bin/cozycast-play
# Pauses the desktop's programs while nobody is in the room.
COPY --chmod=0755 hibernate.py /usr/local/bin/cozycast-hibernate
ENTRYPOINT ["/usr/local/bin/cozycast-entrypoint"]
CMD ["/usr/bin/supervisord", "-c", "/etc/neko/supervisord.conf"]
