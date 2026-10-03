-- Times are unix seconds unless the column name ends in _ms.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT    NOT NULL UNIQUE,          -- lowercase login name
    password_hash TEXT    NOT NULL,
    nickname      TEXT    NOT NULL,
    name_color    TEXT    NOT NULL DEFAULT '#fff',
    avatar        TEXT    NOT NULL DEFAULT '',      -- file name in the avatar dir, '' = default
    admin         INTEGER NOT NULL DEFAULT 0,
    verified      INTEGER NOT NULL DEFAULT 0,
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
    token_hash   BLOB    PRIMARY KEY,               -- sha256 of the cookie value
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE rooms (
    name             TEXT    PRIMARY KEY,
    access           TEXT    NOT NULL DEFAULT 'public'
                     CHECK (access IN ('public', 'account', 'verified', 'invite')),
    hidden           INTEGER NOT NULL DEFAULT 0,    -- hide from the list for people who cannot join
    remote_ownership INTEGER NOT NULL DEFAULT 0,    -- an occupied remote cannot be taken
    center_remote    INTEGER NOT NULL DEFAULT 0,    -- center the pointer when the remote is dropped
    default_remote   INTEGER NOT NULL DEFAULT 0,
    default_image    INTEGER NOT NULL DEFAULT 0,
    default_upload   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE room_permissions (
    room         TEXT    NOT NULL,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    remote       INTEGER NOT NULL DEFAULT 0,
    image        INTEGER NOT NULL DEFAULT 0,
    upload       INTEGER NOT NULL DEFAULT 0,
    trusted      INTEGER NOT NULL DEFAULT 0,
    invited      INTEGER NOT NULL DEFAULT 0,
    invite_name  TEXT    NOT NULL DEFAULT '',
    banned       INTEGER NOT NULL DEFAULT 0,
    banned_until INTEGER,                           -- NULL with banned = 1 means forever
    PRIMARY KEY (room, user_id)
);
CREATE INDEX room_permissions_user ON room_permissions(user_id);

CREATE TABLE anon_bans (
    id           INTEGER PRIMARY KEY,
    room         TEXT    NOT NULL,
    anon_id      TEXT    NOT NULL,
    ip           TEXT    NOT NULL,
    banned_until INTEGER                            -- NULL = forever
);
CREATE INDEX anon_bans_room ON anon_bans(room);

CREATE TABLE invites (
    code       TEXT    PRIMARY KEY,
    room       TEXT    NOT NULL,
    temporary  INTEGER NOT NULL DEFAULT 0,          -- 1 = one-visit access link, 0 = account invite
    name       TEXT    NOT NULL DEFAULT '',
    remote     INTEGER NOT NULL DEFAULT 0,
    image      INTEGER NOT NULL DEFAULT 0,
    upload     INTEGER NOT NULL DEFAULT 0,
    uses       INTEGER NOT NULL DEFAULT 0,
    max_uses   INTEGER,                             -- NULL = unlimited
    expires_at INTEGER,                             -- NULL = never
    created_at INTEGER NOT NULL
);

-- Which accounts redeemed which permanent invite, so re-redeeming is free.
CREATE TABLE invite_redemptions (
    code    TEXT    NOT NULL REFERENCES invites(code) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (code, user_id)
);

CREATE TABLE chat_messages (
    id            INTEGER PRIMARY KEY AUTOINCREMENT, -- monotonic: clients resume after the last id
    room          TEXT    NOT NULL,
    identity      TEXT    NOT NULL,                  -- 'u:<id>' or 'a:<anon id>'
    user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
    nickname      TEXT    NOT NULL,
    name_color    TEXT    NOT NULL,
    anonymous     INTEGER NOT NULL,
    type          TEXT    NOT NULL CHECK (type IN ('text', 'image', 'video')),
    body          TEXT    NOT NULL DEFAULT '',
    media         TEXT    NOT NULL DEFAULT '',       -- file name in the media dir
    edited        INTEGER NOT NULL DEFAULT 0,
    created_at_ms INTEGER NOT NULL
);
CREATE INDEX chat_messages_room ON chat_messages(room, id);
