-- The watchlog per episode: which episodes of a show each account has
-- watched, in place of only the last one. Someone who was at episode 5 has
-- watched 1 to 5.
CREATE TABLE watched_episodes (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id   INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    episode    INTEGER NOT NULL,
    source     TEXT NOT NULL, -- manual | auto
    watched_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, anime_id, episode)
) WITHOUT ROWID;
CREATE INDEX watched_episodes_anime ON watched_episodes(anime_id);

WITH RECURSIVE numbers(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM numbers WHERE n < 9999)
INSERT INTO watched_episodes (user_id, anime_id, episode, source, watched_at)
SELECT p.user_id, p.anime_id, numbers.n, p.source, p.updated_at
FROM watch_progress p JOIN numbers ON numbers.n <= p.episode;

DROP TABLE watch_progress;
