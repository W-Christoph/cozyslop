#!/usr/bin/env python3
"""Tells the CozyCast server which window is in front on the room's desktop.

The server asks (GET /title, every couple of seconds while someone is in the
room) and shows the answer to the room's viewers as their browser tab's
title. Only the server and the desktop itself can reach this port.

supervisord runs this as "nobody", not as the desktop's user: whoever holds
the remote cannot stop or replace it. What they do control is the title
itself, by naming a window; the server treats it as untrusted text.
"""
import subprocess
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 8081


def title():
    try:
        result = subprocess.run(
            ["xdotool", "getactivewindow", "getwindowname"],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=2)
    except (OSError, subprocess.TimeoutExpired):
        return b""
    # No window in front (xdotool fails) is an empty title.
    return result.stdout.strip()[:1024] if result.returncode == 0 else b""


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def do_GET(self):
        if self.path != "/title":
            self.send_error(404)
            return
        body = title()
        self.send_response(200)
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
