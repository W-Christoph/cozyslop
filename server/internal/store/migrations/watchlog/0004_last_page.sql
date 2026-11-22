-- The page the show's latest episode played on in a room's Firefox, and
-- which episode that was: for sites whose addresses do not say the episode,
-- where watch_url stays ''. '' and 0 if none has played.
ALTER TABLE anime ADD COLUMN page_url TEXT NOT NULL DEFAULT '';
ALTER TABLE anime ADD COLUMN page_episode INTEGER NOT NULL DEFAULT 0;
