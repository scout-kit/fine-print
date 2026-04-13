CREATE TABLE photo_statuses (
    id   INT UNSIGNED PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO photo_statuses (id, name) VALUES (1, 'uploaded');
INSERT INTO photo_statuses (id, name) VALUES (2, 'approved');
INSERT INTO photo_statuses (id, name) VALUES (3, 'queued');
INSERT INTO photo_statuses (id, name) VALUES (4, 'printing');
INSERT INTO photo_statuses (id, name) VALUES (5, 'printed');
INSERT INTO photo_statuses (id, name) VALUES (6, 'failed');
INSERT INTO photo_statuses (id, name) VALUES (7, 'rejected');

CREATE TABLE print_job_statuses (
    id   INT UNSIGNED PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO print_job_statuses (id, name) VALUES (1, 'queued');
INSERT INTO print_job_statuses (id, name) VALUES (2, 'printing');
INSERT INTO print_job_statuses (id, name) VALUES (3, 'printed');
INSERT INTO print_job_statuses (id, name) VALUES (4, 'failed');
INSERT INTO print_job_statuses (id, name) VALUES (5, 'canceled');

CREATE TABLE settings (
    id    INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `key` VARCHAR(255) NOT NULL UNIQUE,
    value TEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE projects (
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    brightness  DOUBLE DEFAULT 0.0,
    contrast    DOUBLE DEFAULT 0.0,
    saturation  DOUBLE DEFAULT 0.0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE overlays (
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    project_id  INT UNSIGNED NOT NULL,
    filename    VARCHAR(255) NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    x           DOUBLE DEFAULT 0.0,
    y           DOUBLE DEFAULT 0.0,
    width       DOUBLE DEFAULT 1.0,
    height      DOUBLE DEFAULT 1.0,
    opacity     DOUBLE DEFAULT 1.0,
    z_order     INT DEFAULT 0,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE text_overlays (
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    project_id  INT UNSIGNED NOT NULL,
    text        TEXT NOT NULL,
    font_family VARCHAR(100) DEFAULT 'sans-serif',
    font_size   DOUBLE DEFAULT 24.0,
    color       VARCHAR(20) DEFAULT '#FFFFFF',
    x           DOUBLE DEFAULT 0.5,
    y           DOUBLE DEFAULT 0.5,
    opacity     DOUBLE DEFAULT 1.0,
    z_order     INT DEFAULT 0,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE photos (
    id              INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    project_id      INT UNSIGNED NOT NULL,
    session_id      VARCHAR(100) NOT NULL,
    original_key    VARCHAR(500) NOT NULL,
    preview_key     VARCHAR(500),
    rendered_key    VARCHAR(500),
    original_width  INT UNSIGNED,
    original_height INT UNSIGNED,
    file_size       BIGINT UNSIGNED,
    mime_type       VARCHAR(100),
    status_id       INT UNSIGNED NOT NULL DEFAULT 1,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (status_id) REFERENCES photo_statuses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_photos_status ON photos(status_id);
CREATE INDEX idx_photos_project ON photos(project_id);
CREATE INDEX idx_photos_session ON photos(session_id);

CREATE TABLE photo_transforms (
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    photo_id    INT UNSIGNED NOT NULL UNIQUE,
    crop_x      DOUBLE DEFAULT 0.0,
    crop_y      DOUBLE DEFAULT 0.0,
    crop_width  DOUBLE DEFAULT 1.0,
    crop_height DOUBLE DEFAULT 1.0,
    rotation    DOUBLE DEFAULT 0.0,
    FOREIGN KEY (photo_id) REFERENCES photos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE photo_overrides (
    id                 INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    photo_id           INT UNSIGNED NOT NULL UNIQUE,
    brightness         DOUBLE,
    contrast           DOUBLE,
    saturation         DOUBLE,
    overlay_overrides  JSON,
    text_overrides     JSON,
    FOREIGN KEY (photo_id) REFERENCES photos(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE print_jobs (
    id          INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    photo_id    INT UNSIGNED NOT NULL,
    cups_job_id VARCHAR(100),
    position    INT UNSIGNED NOT NULL,
    status_id   INT UNSIGNED NOT NULL DEFAULT 1,
    error_msg   TEXT,
    attempts    INT UNSIGNED DEFAULT 0,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    printed_at  TIMESTAMP NULL,
    FOREIGN KEY (photo_id) REFERENCES photos(id),
    FOREIGN KEY (status_id) REFERENCES print_job_statuses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_print_jobs_status ON print_jobs(status_id);
CREATE INDEX idx_print_jobs_position ON print_jobs(position);

CREATE TABLE admin_sessions (
    id         INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    token      VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_admin_sessions_token ON admin_sessions(token)
