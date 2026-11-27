#!/usr/bin/env python3
"""Tells the CozyCast server which window is in front on the room's desktop,
and how much memory the room uses.

The server asks (GET /title, every couple of seconds while someone is in the
room) and shows the answer to the room's viewers as their browser tab's
title. It asks GET /memory every few seconds for its health samples, and
GET /memory/detail for an admin's snapshot of what uses it. Only
the server and the desktop itself can reach this port.

supervisord runs this as "nobody", not as the desktop's user: whoever holds
the remote cannot stop or replace it. What they do control is the title
itself, by naming a window; the server treats it as untrusted text.
"""
import json
import os
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


# What the cgroup's memory is made of (memory.stat), and what happened at
# its cap (memory.events); bytes and counts as the kernel has them.
STAT = ("anon", "file", "shmem", "file_mapped", "active_file", "inactive_file",
        "kernel", "kernel_stack", "pagetables", "slab", "sock")
EVENTS = ("high", "max", "oom", "oom_kill")


def memory_detail():
    """memory() with what makes it up, the most it ever was, /dev/shm and
    the programs by name with their resident memory (a page several share
    counts in each). Only the names: never their arguments or windows."""
    base = memory()
    if base is None:
        return None
    def read(name):
        try:
            with open(f"{CGROUP}/{name}") as f:
                return f.read()
        except OSError:
            return ""
    stat = dict(line.split() for line in read("memory.stat").splitlines() if " " in line)
    events = dict(line.split() for line in read("memory.events").splitlines() if " " in line)
    base["stat"] = {k: int(stat[k]) for k in STAT if k in stat}
    base["events"] = {k: int(events[k]) for k in EVENTS if k in events}
    for key, name in (("peak", "memory.peak"), ("swap", "memory.swap.current")):
        if (v := read(name).strip()).isdigit():
            base[key] = int(v)
    try:
        shm = os.statvfs("/dev/shm")
        base["shm"] = {"used": (shm.f_blocks - shm.f_bfree) * shm.f_frsize,
                       "size": shm.f_blocks * shm.f_frsize}
    except OSError:
        pass
    programs = {}
    for pid in os.listdir("/proc"):
        if not pid.isdigit():
            continue
        try:
            with open(f"/proc/{pid}/status") as f:
                fields = dict(line.split(":", 1) for line in f if ":" in line)
        except OSError:
            continue  # ended meanwhile
        name = fields.get("Name", "?").strip()[:32]
        p = programs.setdefault(name, {"name": name, "count": 0, "rss": 0, "anon": 0, "shmem": 0})
        p["count"] += 1
        for key, field in (("rss", "VmRSS"), ("anon", "RssAnon"), ("shmem", "RssShmem")):
            if (v := fields.get(field, "").split()) and v[0].isdigit():
                p[key] += int(v[0]) * 1024  # kB
    base["programs"] = sorted(programs.values(), key=lambda p: -p["rss"])[:20]
    return base


class Handler(BaseHTTPRequestHandler):
    timeout = 5  # a client that stops talking does not keep its thread

    def do_GET(self):
        if self.path == "/title":
            body, kind = title(), "text/plain; charset=utf-8"
        elif self.path == "/memory" and (m := memory()) is not None:
            body, kind = json.dumps(m).encode(), "application/json"
        elif self.path == "/memory/detail" and (m := memory_detail()) is not None:
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
