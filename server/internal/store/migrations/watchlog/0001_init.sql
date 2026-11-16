-- The watchlog: which episodes of which anime each account has watched
-- (docs/architecture.md, "Watchlog"). Its own database, beside the main
-- one: user_id is users.id over there, which nothing here can check.
-- Shows are AniList entries, keyed by their AniList id.
CREATE TABLE anime (
    id            INTEGER PRIMARY KEY,
    title         TEXT NOT NULL,
    title_english TEXT NOT NULL DEFAULT '',
    episodes      INTEGER,
    cover_url     TEXT NOT NULL DEFAULT '',
    added_at      INTEGER NOT NULL
);

-- File names already matched to a show: the name's title part, normalised.
CREATE TABLE anime_aliases (
    alias    TEXT PRIMARY KEY,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE
);
CREATE INDEX anime_aliases_anime ON anime_aliases(anime_id);

CREATE TABLE watched_episodes (
    user_id    INTEGER NOT NULL,
    anime_id   INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    episode    INTEGER NOT NULL,
    source     TEXT NOT NULL, -- manual | auto
    watched_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, anime_id, episode)
) WITHOUT ROWID;
CREATE INDEX watched_episodes_anime ON watched_episodes(anime_id);

-- Someone who is not following a show any more, though they watched some
-- of it. The others do not wait for them.
CREATE TABLE watch_ignored (
    user_id  INTEGER NOT NULL,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, anime_id)
) WITHOUT ROWID;
CREATE INDEX watch_ignored_anime ON watch_ignored(anime_id);
