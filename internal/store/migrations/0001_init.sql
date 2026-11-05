CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);



-- ---------- Pocket Places: everything below is per person ----------

-- A place worth remembering: a restaurant, a café, a bar, a museum, a beach.
CREATE TABLE places (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  kind           TEXT NOT NULL DEFAULT 'restaurant',  -- see store/kinds.go
  cuisine        TEXT NOT NULL DEFAULT '',            -- "Bosnian, grill", "specialty coffee"
  status         TEXT NOT NULL DEFAULT 'want',        -- want | been
  favourite      INTEGER NOT NULL DEFAULT 0,
  closed         INTEGER NOT NULL DEFAULT 0,          -- closed for good: kept, never suggested
  rating         REAL NOT NULL DEFAULT 0,             -- 0 = not rated, else 1..5 in halves
  price          INTEGER NOT NULL DEFAULT 0,          -- 0 = unknown, 1..4 (€ to €€€€)
  address        TEXT NOT NULL DEFAULT '',
  city           TEXT NOT NULL DEFAULT '',
  country        TEXT NOT NULL DEFAULT '',            -- ISO 3166 alpha-2
  lat            REAL,
  lng            REAL,
  phone          TEXT NOT NULL DEFAULT '',
  website        TEXT NOT NULL DEFAULT '',
  map_url        TEXT NOT NULL DEFAULT '',            -- Google Maps / OpenStreetMap link
  hours          TEXT NOT NULL DEFAULT '',            -- opening hours as text
  tags           TEXT NOT NULL DEFAULT '',            -- "date night, terrace, kids"
  notes          TEXT NOT NULL DEFAULT '',
  recommended_by TEXT NOT NULL DEFAULT '',
  contact_id     INTEGER,                             -- who recommended it, in Pocket Contacts
  currency       TEXT NOT NULL DEFAULT '',            -- what you pay in there; '' = your currency
  source         TEXT NOT NULL DEFAULT '',            -- '' by hand | import | google | <app slug>
  source_ref     TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX places_user ON places(user_id, status, name);
CREATE INDEX places_city ON places(user_id, country, city);
CREATE UNIQUE INDEX places_source ON places(user_id, source, source_ref) WHERE source_ref <> '';

-- A time you went (or plan to go): when, how it was, with whom, what it cost.
CREATE TABLE visits (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  place_id        INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  date            TEXT NOT NULL,                      -- YYYY-MM-DD
  time            TEXT NOT NULL DEFAULT '',           -- HH:MM
  planned         INTEGER NOT NULL DEFAULT 0,         -- a plan, not been yet
  rating          REAL NOT NULL DEFAULT 0,
  notes           TEXT NOT NULL DEFAULT '',           -- what we had, how it was
  with_whom       TEXT NOT NULL DEFAULT '',           -- names, comma separated
  minutes         INTEGER NOT NULL DEFAULT 0,         -- how long (Pocket Movement)
  spent           INTEGER NOT NULL DEFAULT 0,         -- minor units of currency
  currency        TEXT NOT NULL DEFAULT '',
  money_tx_id     INTEGER,                            -- the transaction in Pocket Money
  money_booked    INTEGER NOT NULL DEFAULT 0,         -- 1 = Places created it, 0 = an existing one was linked
  split_group_id  INTEGER,                            -- the bill split in Pocket Split
  split_expense_id INTEGER,
  cal_event_id    INTEGER,                            -- a planned visit on Pocket Calendar
  trip_id         INTEGER,                            -- the Pocket Travel trip it was part of
  trip_name       TEXT NOT NULL DEFAULT '',
  source          TEXT NOT NULL DEFAULT '',           -- '' by hand | movement | money | import | <app slug>
  source_ref      TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX visits_user ON visits(user_id, date);
CREATE INDEX visits_place ON visits(user_id, place_id, date);
CREATE UNIQUE INDEX visits_source ON visits(user_id, source, source_ref) WHERE source_ref <> '';

-- Your own lists: "Date night", "Coffee in Sarajevo", "Lisbon in May".
CREATE TABLE lists (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  emoji       TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  trip_id     INTEGER,                                -- a Pocket Travel trip this list is for
  trip_name   TEXT NOT NULL DEFAULT '',
  sort        INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX lists_user ON lists(user_id, sort, name);

CREATE TABLE list_places (
  list_id   INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
  place_id  INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  note      TEXT NOT NULL DEFAULT '',
  added_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (list_id, place_id)
);
CREATE INDEX list_places_place ON list_places(user_id, place_id);

-- "You were at Café Tito for an hour — how was it?": a stay seen by Pocket
-- Movement and/or a payment in Pocket Money at one of your places on one
-- day. One per place and day; a dismissed one never comes back.
CREATE TABLE inbox (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  place_id    INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  date        TEXT NOT NULL,
  time        TEXT NOT NULL DEFAULT '',
  minutes     INTEGER NOT NULL DEFAULT 0,
  movement_id INTEGER,                                -- the visit in Pocket Movement
  amount      INTEGER NOT NULL DEFAULT 0,             -- minor units
  currency    TEXT NOT NULL DEFAULT '',
  money_tx_id INTEGER,
  payee       TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'open',           -- open | done | dismissed
  notified    INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, place_id, date)
);
CREATE INDEX inbox_user ON inbox(user_id, status, date);
