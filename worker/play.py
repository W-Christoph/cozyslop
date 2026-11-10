#!/usr/bin/env python3
"""Plays a file of the room desktop's Downloads folder in VLC, fullscreen.

The CozyCast server asks (POST /play?name=<file>) on behalf of someone in
the room who may use the remote; a file played before is closed first.
GET /status says what that VLC is playing and how far it is, from VLC's own
web interface, which only listens on this machine and wants a password made
up for each VLC. While that VLC plays nothing it says what Firefox plays
instead, as Firefox tells the desktop's media keys (MPRIS, on the session
bus): the page's title and address, not a file.

supervisord runs this as the desktop's user: VLC has to open on their
desktop, and whoever holds the remote can start it by hand anyway. Every
request must carry the room's neko admin token, which only the server and
the desktop's user know: a web page open in the room's browser reaches this
port too, and must not be able to use it.
"""
import base64
import hmac
import json
import os
import re
import secrets
import subprocess
import threading
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

PORT = 8082
DIR = os.environ.get("NEKO_FILETRANSFER_DIR") or os.path.expanduser("~/Downloads")
TOKEN = os.environ.get("NEKO_SESSION_API_TOKEN", "")
VLC_PORT = 8084  # VLC's web interface, on 127.0.0.1 only

lock = threading.Lock()
player = None  # the VLC started last, replaced by the next one
password = ""  # of its web interface


def path_of(name):
    """The file called name in the folder itself, or None."""
    if not name or name in (".", "..") or "/" in name or "\0" in name:
        return None
    path = os.path.join(DIR, name)
    # A link may point out of the folder: it is left alone.
    if os.path.islink(path) or not os.path.isfile(path):
        return None
    return path


def play(path):
    global player, password
    with lock:
        if player and player.poll() is None:
            player.terminate()
            try:
                player.wait(timeout=5)
            except subprocess.TimeoutExpired:
                player.kill()
                player.wait()
        password = secrets.token_urlsafe(24)
        player = subprocess.Popen(
            ["vlc", "--fullscreen", "--no-qt-privacy-ask", "--no-qt-error-dialogs",
             "--extraintf", "http", "--http-host", "127.0.0.1", "--http-port", str(VLC_PORT),
             "--http-password", password, "--", path],
            stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            start_new_session=True)
        # Reap it when it is closed on the desktop.
        threading.Thread(target=player.wait, daemon=True).start()


def status():
    """What plays on the desktop: the VLC started last, or else Firefox."""
    vlc = vlc_status()
    if vlc["state"] in ("playing", "unknown"):
        return vlc
    page = firefox_status()
    if page and (page["state"] == "playing" or vlc["state"] == "stopped"):
        return page
    return vlc


def vlc_status():
    """What the VLC started last plays: state (playing, paused or stopped),
    time and length in seconds, and the file's name."""
    with lock:
        running = player is not None and player.poll() is None
        secret = password
    if not running:
        return {"state": "stopped"}
    req = urllib.request.Request(f"http://127.0.0.1:{VLC_PORT}/requests/status.json")
    req.add_header("Authorization", "Basic " + base64.b64encode((":" + secret).encode()).decode())
    try:
        with urllib.request.urlopen(req, timeout=2) as res:
            vlc = json.load(res)
    except (OSError, ValueError):
        return {"state": "unknown"}  # starting, or busy
    meta = (vlc.get("information") or {}).get("category", {}).get("meta", {})
    return {"state": vlc.get("state", "stopped"), "time": vlc.get("time", 0),
            "length": vlc.get("length", 0), "file": meta.get("filename", "")}


def buses():
    """The session buses a Firefox of the desktop's user may be on: None for
    the one found through the display, where supervisord's Firefox is, and
    the address a Firefox opened from the desktop's menu was given."""
    found = {None}
    for pid in os.listdir("/proc"):
        try:
            with open(f"/proc/{pid}/comm") as f:
                if not f.read().startswith("firefox"):
                    continue
            with open(f"/proc/{pid}/environ", "rb") as f:
                env = f.read().split(b"\0")
        except (OSError, ValueError):
            continue  # gone, or not ours
        for var in env:
            if var.startswith(b"DBUS_SESSION_BUS_ADDRESS="):
                found.add(var.split(b"=", 1)[1].decode(errors="replace"))
    return found


def dbus(bus, dest, path, method, *args):
    """dbus-send's printed reply, or "" if there was none."""
    env = dict(os.environ)
    env.pop("DBUS_SESSION_BUS_ADDRESS", None)
    if bus:
        env["DBUS_SESSION_BUS_ADDRESS"] = bus
    try:
        result = subprocess.run(
            ["dbus-send", "--session", "--print-reply", "--reply-timeout=1000", "--dest=" + dest, path, method, *args],
            env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=2)
    except (OSError, subprocess.TimeoutExpired):
        return ""
    return result.stdout.decode(errors="replace") if result.returncode == 0 else ""


def firefox_status():
    """What Firefox plays, or None: state, time and length in seconds, and
    the title and address of the page. A playing Firefox wins over a paused
    one. Both texts are the page's to choose."""
    best = None
    for bus in buses():
        names = dbus(bus, "org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus.ListNames")
        for name in re.findall(r'"(org\.mpris\.MediaPlayer2\.firefox[^"]*)"', names):
            reply = dbus(bus, name, "/org/mpris/MediaPlayer2", "org.freedesktop.DBus.Properties.GetAll",
                         "string:org.mpris.MediaPlayer2.Player")

            def value(key, kind):
                m = re.search(r'string "%s"\s+variant\s+%s (.*)' % (re.escape(key), kind), reply)
                return m.group(1) if m else ""

            def seconds(key):
                v = value(key, "int64")
                return int(v) // 1000000 if v.isdigit() else 0

            state = value("PlaybackStatus", "string").strip('"').lower()
            if state not in ("playing", "paused"):
                continue
            found = {"state": state, "source": "firefox", "time": seconds("Position"), "length": seconds("mpris:length"),
                     "title": value("xesam:title", "string")[1:-1][:300], "url": value("xesam:url", "string")[1:-1][:500]}
            if state == "playing":
                return found
            best = best or found
    return best


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def allowed(self):
        given = self.headers.get("Authorization", "")
        return TOKEN and hmac.compare_digest(given.encode(), ("Bearer " + TOKEN).encode())

    def do_GET(self):
        if urlsplit(self.path).path != "/status":
            return self.answer(404, "Unknown action.")
        if not self.allowed():
            return self.answer(403, "Not allowed.")
        self.answer(200, json.dumps(status()), "application/json")

    def do_POST(self):
        url = urlsplit(self.path)
        if url.path != "/play":
            return self.answer(404, "Unknown action.")
        if not self.allowed():
            return self.answer(403, "Not allowed.")
        path = path_of(parse_qs(url.query).get("name", [""])[0])
        if path is None:
            return self.answer(404, "That file is not in Downloads.")
        try:
            play(path)
        except OSError as e:
            return self.answer(500, e.strerror or "Failed.")
        self.answer(204, "")

    def answer(self, code, text, kind="text/plain; charset=utf-8"):
        body = text.encode()
        self.send_response(code)
        self.send_header("Content-Type", kind)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        pass


if __name__ == "__main__":
    server = ThreadingHTTPServer(("", PORT), Handler)
    server.daemon_threads = True
    server.serve_forever()
