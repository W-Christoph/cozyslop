-- "Always center the remote" is gone as a room setting: the pointer is
-- centered with the "Drop and center Remote" button when someone wants it.
ALTER TABLE rooms DROP COLUMN center_remote;
