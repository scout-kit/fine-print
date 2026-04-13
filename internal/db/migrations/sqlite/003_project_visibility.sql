CREATE TABLE project_visibilities (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO project_visibilities (id, name) VALUES (1, 'public');
INSERT INTO project_visibilities (id, name) VALUES (2, 'hidden');
INSERT INTO project_visibilities (id, name) VALUES (3, 'private');

ALTER TABLE projects ADD COLUMN visibility_id INTEGER NOT NULL DEFAULT 1 REFERENCES project_visibilities(id);
ALTER TABLE projects ADD COLUMN slug TEXT
