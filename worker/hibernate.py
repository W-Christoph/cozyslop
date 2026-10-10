#!/usr/bin/env python3
"""Pauses and continues the programs on the room's desktop.

The CozyCast server asks (POST /freeze) once nobody has been in the room
for a while, so that a video left playing there does not keep the machine
busy, and (POST /thaw) when someone comes in. A paused program keeps its
windows and everything in them; it only stops running. neko, the X server
and the sound server go on, so the room stays reachable and whoever joins
has a picture at once.

supervisord runs this as the desktop's user, which is all it takes to
signal that user's programs. Every request must carry the room's neko admin
token, which only the server and the desktop's user know: a web page open
in the room's browser reaches this port too, and must not be able to use it.
"""
import hmac
import os
import signal
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 8083
TOKEN = os.environ.get("NEKO_SESSION_API_TOKEN", "")
KEEP = {"neko", "Xorg", "pulseaudio"}

lock = threading.Lock()


def programs():
    """The desktop user's processes apart from KEEP and this one: (pid, state)."""
    found = []
    for name in os.listdir("/proc"):
        if not name.isdigit() or int(name) == os.getpid():
            continue
        try:
            with open(f"/proc/{name}/status") as f:
                status = dict(line.split(":\t", 1) for line in f if ":\t" in line)
        except OSError:
            continue  # it ended in the meantime
        if int(status["Uid"].split()[0]) == os.getuid() and status["Name"].strip() not in KEEP:
            found.append((int(name), status["State"].split()[0]))
    return sorted(found)


def send(pid, sig):
    try:
        os.kill(pid, sig)
    except ProcessLookupError:
        pass


def freeze():
    # Again until nothing is left running: a program may start another
    # while the first ones are being stopped. Parents before their children
    # (lowest pid first) and, in thaw, children before their parents: a
    # shell that sees its program stop would take it for put in the
    # background.
    with lock:
        for _ in range(40):
            running = [pid for pid, state in programs() if state not in "Tt"]
            if not running:
                return True
            for pid in running:
                send(pid, signal.SIGSTOP)
            time.sleep(0.05)
        return False


def thaw():
    # Again while something is stopped: a program that learns on waking
    # that its child had stopped may stop itself in answer (script(1)).
    with lock:
        for _ in range(10):
            stopped = [pid for pid, state in programs() if state in "Tt"]
            if not stopped:
                break
            for pid in reversed(stopped):
                send(pid, signal.SIGCONT)
            time.sleep(0.05)
    return True


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def do_POST(self):
        action = {"/freeze": freeze, "/thaw": thaw}.get(self.path)
        if action is None:
            return self.answer(404, "Unknown action.")
        given = self.headers.get("Authorization", "")
        if not TOKEN or not hmac.compare_digest(given.encode(), ("Bearer " + TOKEN).encode()):
            return self.answer(403, "Not allowed.")
        if not action():
            return self.answer(500, "Programs kept starting.")
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
