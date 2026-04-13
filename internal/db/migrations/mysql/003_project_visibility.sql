CREATE TABLE project_visibilities (
    id   INT UNSIGNED PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO project_visibilities (id, name) VALUES (1, 'public');
INSERT INTO project_visibilities (id, name) VALUES (2, 'hidden');
INSERT INTO project_visibilities (id, name) VALUES (3, 'private');

ALTER TABLE projects ADD COLUMN visibility_id INT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE projects ADD COLUMN slug VARCHAR(100);
ALTER TABLE projects ADD FOREIGN KEY (visibility_id) REFERENCES project_visibilities(id)
