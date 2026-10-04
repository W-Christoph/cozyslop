-- Logins carried over from the old CozyCast. A browser that was logged in
-- there still holds a refresh token; presenting it starts a session here,
-- once, without a password.
CREATE TABLE legacy_logins (
    token_hash BLOB    PRIMARY KEY,               -- sha256 of the old refresh token
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX legacy_logins_user ON legacy_logins(user_id);
