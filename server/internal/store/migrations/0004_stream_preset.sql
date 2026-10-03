-- Stream ids gained the x264 preset (b<kbps>-s<percent>-<preset>). Rooms
-- keep their bitrate and size with the default preset; an id the worker
-- does not offer falls back to neko's default stream.
UPDATE rooms SET stream = stream || '-veryfast'
WHERE stream GLOB 'b[0-9]*-s[0-9]*' AND stream NOT GLOB 'b*-s*-*';
