-- The watchlog: how far each account has watched which anime
-- (docs/architecture.md, "Watchlog"). Shows are AniList entries, keyed by
-- their AniList id.
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

CREATE TABLE watch_progress (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id   INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    episode    INTEGER NOT NULL,
    source     TEXT NOT NULL, -- manual | auto
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, anime_id)
);
CREATE INDEX watch_progress_anime ON watch_progress(anime_id);
CREATE INDEX anime_aliases_anime ON anime_aliases(anime_id);
