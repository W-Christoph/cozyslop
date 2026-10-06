CREATE TABLE registered_rooms (
    name       TEXT PRIMARY KEY,
    neko_url   TEXT NOT NULL,
    neko_token TEXT NOT NULL,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL
);
