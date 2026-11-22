-- Where a show is watched: the address of one of its episodes' pages with
-- "{ep}" where the episode number goes, learned from what played in a
-- room's Firefox or set by hand; '' if nobody knows.
ALTER TABLE anime ADD COLUMN watch_url TEXT NOT NULL DEFAULT '';
-- The next episode to air and when (unix seconds), as AniList says; NULL
-- while none is announced.
ALTER TABLE anime ADD COLUMN next_episode INTEGER;
ALTER TABLE anime ADD COLUMN next_airing_at INTEGER;
