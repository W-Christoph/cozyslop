#!/usr/bin/env python3
"""Tells the CozyCast server which window is in front on the room's desktop,
and how much memory the room uses.

The server asks (GET /title, every couple of seconds while someone is in the
room) and shows the answer to the room's viewers as their browser tab's
title. It asks GET /memory every few seconds for its health samples. Only
the server and the desktop itself can reach this port.

supervisord runs this as "nobody", not as the desktop's user: whoever holds
the remote cannot stop or replace it. What they do control is the title
itself, by naming a window; the server treats it as untrusted text.
"""
import json
import subprocess
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 8081
CGROUP = "/sys/fs/cgroup"


def title():
    try:
        result = subprocess.run(
            ["xdotool", "getactivewindow", "getwindowname"],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=2)
    except (OSError, subprocess.TimeoutExpired):
        return b""
    # No window in front (xdotool fails) is an empty title.
    return result.stdout.strip()[:1024] if result.returncode == 0 else b""


def memory():
    """The room container's memory, as its cgroup (v2) counts it: what is in
    use without the file cache the kernel drops first (as `docker stats`
    shows it), the cap (None without one) and how many programs the kernel
    ended for lack of memory. None where the container has no such cgroup."""
    def read(name):
        with open(f"{CGROUP}/{name}") as f:
            return f.read()
    try:
        current = int(read("memory.current"))
        cap = read("memory.max").strip()
        stat = dict(line.split() for line in read("memory.stat").splitlines())
        events = dict(line.split() for line in read("memory.events").splitlines())
        return {"used": max(0, current - int(stat.get("inactive_file", 0))),
                "limit": None if cap == "max" else int(cap),
                "oomKills": int(events.get("oom_kill", 0))}
    except (OSError, ValueError):
        return None


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def do_GET(self):
        if self.path == "/title":
            body, kind = title(), "text/plain; charset=utf-8"
        elif self.path == "/memory" and (m := memory()) is not None:
            body, kind = json.dumps(m).encode(), "application/json"
        else:
            self.send_error(404)
            return
        self.send_response(200)
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
