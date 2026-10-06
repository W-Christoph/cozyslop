CREATE TABLE password_resets (
    token_hash BLOB PRIMARY KEY CHECK (length(token_hash) = 32),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at INTEGER
);
CREATE INDEX password_resets_user ON password_resets(user_id);
CREATE INDEX password_resets_expiry ON password_resets(expires_at);
