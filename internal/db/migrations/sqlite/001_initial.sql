CREATE TABLE photo_statuses (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO photo_statuses (id, name) VALUES (1, 'uploaded');
INSERT INTO photo_statuses (id, name) VALUES (2, 'approved');
INSERT INTO photo_statuses (id, name) VALUES (3, 'queued');
INSERT INTO photo_statuses (id, name) VALUES (4, 'printing');
INSERT INTO photo_statuses (id, name) VALUES (5, 'printed');
INSERT INTO photo_statuses (id, name) VALUES (6, 'failed');
INSERT INTO photo_statuses (id, name) VALUES (7, 'rejected');

CREATE TABLE print_job_statuses (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO print_job_statuses (id, name) VALUES (1, 'queued');
INSERT INTO print_job_statuses (id, name) VALUES (2, 'printing');
INSERT INTO print_job_statuses (id, name) VALUES (3, 'printed');
INSERT INTO print_job_statuses (id, name) VALUES (4, 'failed');
INSERT INTO print_job_statuses (id, name) VALUES (5, 'canceled');

CREATE TABLE settings (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    key   TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL
);

CREATE TABLE projects (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    brightness  REAL DEFAULT 0.0,
    contrast    REAL DEFAULT 0.0,
    saturation  REAL DEFAULT 0.0
);

CREATE TABLE overlays (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    x           REAL DEFAULT 0.0,
    y           REAL DEFAULT 0.0,
    width       REAL DEFAULT 1.0,
    height      REAL DEFAULT 1.0,
    opacity     REAL DEFAULT 1.0,
    z_order     INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE text_overlays (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    text        TEXT NOT NULL,
    font_family TEXT DEFAULT 'sans-serif',
    font_size   REAL DEFAULT 24.0,
    color       TEXT DEFAULT '#FFFFFF',
    x           REAL DEFAULT 0.5,
    y           REAL DEFAULT 0.5,
    opacity     REAL DEFAULT 1.0,
    z_order     INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE photos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id      INTEGER NOT NULL REFERENCES projects(id),
    session_id      TEXT NOT NULL,
    original_key    TEXT NOT NULL,
    preview_key     TEXT,
    rendered_key    TEXT,
    original_width  INTEGER,
    original_height INTEGER,
    file_size       INTEGER,
    mime_type       TEXT,
    status_id       INTEGER NOT NULL DEFAULT 1 REFERENCES photo_statuses(id),
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_photos_status ON photos(status_id);
CREATE INDEX idx_photos_project ON photos(project_id);
CREATE INDEX idx_photos_session ON photos(session_id);

CREATE TABLE photo_transforms (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    photo_id    INTEGER NOT NULL UNIQUE REFERENCES photos(id) ON DELETE CASCADE,
    crop_x      REAL DEFAULT 0.0,
    crop_y      REAL DEFAULT 0.0,
    crop_width  REAL DEFAULT 1.0,
    crop_height REAL DEFAULT 1.0,
    rotation    REAL DEFAULT 0.0
);

CREATE TABLE photo_overrides (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    photo_id           INTEGER NOT NULL UNIQUE REFERENCES photos(id) ON DELETE CASCADE,
    brightness         REAL,
    contrast           REAL,
    saturation         REAL,
    overlay_overrides  TEXT,
    text_overrides     TEXT
);

CREATE TABLE print_jobs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    photo_id    INTEGER NOT NULL REFERENCES photos(id),
    cups_job_id TEXT,
    position    INTEGER NOT NULL,
    status_id   INTEGER NOT NULL DEFAULT 1 REFERENCES print_job_statuses(id),
    error_msg   TEXT,
    attempts    INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    printed_at  DATETIME
);

CREATE INDEX idx_print_jobs_status ON print_jobs(status_id);
CREATE INDEX idx_print_jobs_position ON print_jobs(position);

CREATE TABLE admin_sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token      TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX idx_admin_sessions_token ON admin_sessions(token)
