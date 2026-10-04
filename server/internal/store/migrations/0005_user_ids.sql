-- Account identity keys outlive deleted users in chat; never reuse their ids.
-- The migration runner disables foreign keys and checks them before commit.
CREATE TABLE users_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    nickname      TEXT    NOT NULL,
    name_color    TEXT    NOT NULL DEFAULT '#fff',
    avatar        TEXT    NOT NULL DEFAULT '',
    admin         INTEGER NOT NULL DEFAULT 0,
    verified      INTEGER NOT NULL DEFAULT 0,
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

INSERT INTO users_new (id, username, password_hash, nickname, name_color,
                       avatar, admin, verified, disabled, created_at)
SELECT id, username, password_hash, nickname, name_color,
       avatar, admin, verified, disabled, created_at
FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

-- Include ids already freed before this migration but still retained in chat.
DELETE FROM sqlite_sequence WHERE name = 'users';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'users', max(id) FROM (
    SELECT 0 AS id
    UNION ALL SELECT id FROM users
    UNION ALL SELECT CAST(substr(identity, 3) AS INTEGER) FROM chat_messages
    WHERE identity GLOB 'u:[0-9]*' AND substr(identity, 3) NOT GLOB '*[^0-9]*'
);
