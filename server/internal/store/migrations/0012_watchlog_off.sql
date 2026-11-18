-- An account that left the watchlog: rooms do not follow what it watches,
-- and the watchlog leaves it out (docs/architecture.md, "Watchlog"). What
-- it had watched is kept, for when it takes part again.
ALTER TABLE users ADD COLUMN watchlog_off INTEGER NOT NULL DEFAULT 0;
