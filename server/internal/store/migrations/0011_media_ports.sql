-- The server's public port (UDP and TCP) for a paired room's media,
-- forwarded through the tunnel (docs/home-hosting.md).
ALTER TABLE registered_rooms ADD COLUMN media_port INTEGER;
CREATE UNIQUE INDEX registered_rooms_media_port ON registered_rooms(media_port) WHERE media_port IS NOT NULL;
