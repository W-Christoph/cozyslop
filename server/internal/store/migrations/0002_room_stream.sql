-- Stream settings per room. screen '' = neko's configured default
-- (NEKO_DESKTOP_SCREEN); quality names a capture pipeline (compose.yaml).
ALTER TABLE rooms ADD COLUMN screen TEXT NOT NULL DEFAULT '';
ALTER TABLE rooms ADD COLUMN quality TEXT NOT NULL DEFAULT 'medium'
    CHECK (quality IN ('high', 'medium', 'low'));
