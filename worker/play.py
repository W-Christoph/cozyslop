#!/usr/bin/env python3
"""Plays a file of the room desktop's Downloads folder in VLC, fullscreen.

The CozyCast server asks (POST /play?name=<file>) on behalf of someone in
the room who may use the remote; a file played before is closed first.

supervisord runs this as the desktop's user: VLC has to open on their
desktop, and whoever holds the remote can start it by hand anyway. Every
request must carry the room's neko admin token, which only the server and
the desktop's user know: a web page open in the room's browser reaches this
port too, and must not be able to use it.
"""
import hmac
import os
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

PORT = 8082
DIR = os.environ.get("NEKO_FILETRANSFER_DIR") or os.path.expanduser("~/Downloads")
TOKEN = os.environ.get("NEKO_SESSION_API_TOKEN", "")

lock = threading.Lock()
player = None  # the VLC started last, replaced by the next one


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
    global player
    with lock:
        if player and player.poll() is None:
            player.terminate()
            try:
                player.wait(timeout=5)
            except subprocess.TimeoutExpired:
                player.kill()
                player.wait()
        player = subprocess.Popen(
            ["vlc", "--fullscreen", "--no-qt-privacy-ask", "--no-qt-error-dialogs", "--", path],
            stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            start_new_session=True)
        # Reap it when it is closed on the desktop.
        threading.Thread(target=player.wait, daemon=True).start()


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def do_POST(self):
        url = urlsplit(self.path)
        if url.path != "/play":
            return self.answer(404, "Unknown action.")
        given = self.headers.get("Authorization", "")
        if not TOKEN or not hmac.compare_digest(given.encode(), ("Bearer " + TOKEN).encode()):
            return self.answer(403, "Not allowed.")
        path = path_of(parse_qs(url.query).get("name", [""])[0])
        if path is None:
            return self.answer(404, "That file is not in Downloads.")
        try:
            play(path)
        except OSError as e:
            return self.answer(500, e.strerror or "Failed.")
        self.answer(204, "")

    def answer(self, status, text):
        body = text.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        pass


if __name__ == "__main__":
    server = ThreadingHTTPServer(("", PORT), Handler)
    server.daemon_threads = True
    server.serve_forever()
