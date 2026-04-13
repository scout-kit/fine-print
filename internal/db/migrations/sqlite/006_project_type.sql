CREATE TABLE project_types (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO project_types (id, name) VALUES (1, 'standard');
INSERT INTO project_types (id, name) VALUES (2, 'booth');

ALTER TABLE projects ADD COLUMN project_type_id INTEGER NOT NULL DEFAULT 1 REFERENCES project_types(id);
ALTER TABLE projects ADD COLUMN booth_countdown INTEGER NOT NULL DEFAULT 0
