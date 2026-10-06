-- Rooms on other machines, connected through the server's WireGuard tunnel
-- (docs/home-hosting.md). Empty for rooms registered by address.
ALTER TABLE registered_rooms ADD COLUMN node_key TEXT;
ALTER TABLE registered_rooms ADD COLUMN tunnel_address TEXT;
-- Where the node's packets last came from, so a restarted server can reach
-- it before it reconnects on its own.
ALTER TABLE registered_rooms ADD COLUMN node_endpoint TEXT;
CREATE UNIQUE INDEX registered_rooms_node_key ON registered_rooms(node_key) WHERE node_key IS NOT NULL;
CREATE UNIQUE INDEX registered_rooms_tunnel_address ON registered_rooms(tunnel_address) WHERE tunnel_address IS NOT NULL;
