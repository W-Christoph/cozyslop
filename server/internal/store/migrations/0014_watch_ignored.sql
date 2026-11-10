-- The watchlog: someone who is not following a show any more, though they
-- watched some of it. The others do not wait for them.
CREATE TABLE watch_ignored (
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, anime_id)
) WITHOUT ROWID;
CREATE INDEX watch_ignored_anime ON watch_ignored(anime_id);
