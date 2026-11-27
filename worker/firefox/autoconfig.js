// Firefox reads mozilla.cfg (CozyCast's defaults) at start. Without the
// sandbox, that file may also run code in Firefox: it unloads tabs nobody
// looks at, by the room's memory. Only root can change either file.
pref("general.config.obscure_value", 0);
pref("general.config.filename", "mozilla.cfg");
pref("general.config.sandbox_enabled", false);
