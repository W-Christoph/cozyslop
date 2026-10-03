# Ideas and later work

Things that are possible but not built yet. Most are neko features that
CozyCast does not use; none of them needs a patched neko.

## Unused neko features

- **Automatic quality per viewer.** neko can estimate each viewer's bandwidth
  and switch them between the stream pipelines on its own
  (`NEKO_WEBRTC_ESTIMATOR_ENABLED` and friends; viewers opt in with
  `auto: true` in their `signal/video` request).
  Today the room admin picks one stream for everyone. Viewers on weak
  connections would get a smaller stream instead of stuttering. Every
  extra pipeline in use is an extra encoder, and the server's neko proxy
  pins all viewers to the room's stream (`neko.PinStream`); this would be a
  room setting that relaxes it.
- **Personal quality choice.** Let each viewer pick a lower stream than the
  room default for themselves (`signal/video` per session). The pipelines
  already exist, but each one in use is an extra encoder: it needs a room
  setting that lets the proxy pass the viewer's choice (see above).
- **JPEG screencast fallback** (`NEKO_CAPTURE_SCREENCAST_*`). A slow image
  feed over HTTP for viewers whose WebRTC fails entirely (strict firewalls),
  or for thumbnails of a room in a room list.
- **Downloads from the room.** neko's file transfer plugin also lists and
  serves the desktop's Downloads folder (a `filetransfer/update` WebSocket
  event with the file list, `GET /api/filetransfer?filename=` to download). CozyCast
  only uses the upload half, and its proxy only lets `POST` through. Needs a
  download permission and a small file list in the UI.
- **RTMP broadcast** (`NEKO_CAPTURE_BROADCAST_*`, `/api/room/broadcast`).
  Stream the room to Twitch/YouTube/an RTMP server, started by an admin.
  Costs one extra encode while it runs.
- **Microphone and webcam** (`NEKO_CAPTURE_MICROPHONE_*`,
  `NEKO_CAPTURE_WEBCAM_*`). The host's mic or camera becomes a device inside
  the desktop, e.g. for a video call in the room's browser.
- **Other viewers' cursors** (`NEKO_SESSION_INACTIVE_CURSORS`). Shows where
  non-hosts point on the desktop. Needs drawing the cursors over the video.
- **Hardware encoding.** neko publishes Intel (VA-API) and NVIDIA (NVENC)
  images. A worker variant built on them would move encoding off the CPU on
  hosts that have a GPU; the pipelines in `worker/entrypoint.sh` would switch
  to `vaapih264enc` / `nvh264enc`.

## Networking (deferred)

- **Hide the server's IP / relay media.** WebRTC media goes straight to the
  room container's public IP. A TURN server (coturn) with TURN-only ICE
  would put a relay in front of it, and also helps viewers behind strict
  NATs and firewalls.
- **Hosting at home.** Running CozyCast on a home PC exposes the home IP and
  needs port forwarding (HTTPS plus the media port). Options: a VPS running
  only coturn and a reverse proxy, a WireGuard tunnel to a VPS, or a mesh VPN
  (Tailscale, ZeroTier) for private groups.
- **Isolate the room from the LAN.** The room's browser can reach whatever
  the host's network can, including a home LAN and router. Put room
  containers on their own Docker network with egress rules that block
  private address ranges.

## Server

- A CPU benchmark script for real servers (one room, video playing, report
  CPU per stream setting), and measurements of fullscreen playback and
  several viewers over the internet.
