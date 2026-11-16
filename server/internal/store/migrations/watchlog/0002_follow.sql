-- How someone follows a show, where it is not the usual way: casual, they
-- watch along when they are there and nobody waits for them; dropped, they
-- have stopped. Without a row the others wait for them. Replaces
-- watch_ignored, which knew dropped only.
CREATE TABLE watch_follow (
    user_id  INTEGER NOT NULL,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    mode     TEXT NOT NULL CHECK (mode IN ('casual', 'dropped')),
    PRIMARY KEY (user_id, anime_id)
) WITHOUT ROWID;
CREATE INDEX watch_follow_anime ON watch_follow(anime_id);

INSERT INTO watch_follow (user_id, anime_id, mode) SELECT user_id, anime_id, 'dropped' FROM watch_ignored;
DROP TABLE watch_ignored;
