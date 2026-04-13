CREATE TABLE project_types (
    id   INT UNSIGNED PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO project_types (id, name) VALUES (1, 'standard');
INSERT INTO project_types (id, name) VALUES (2, 'booth');

ALTER TABLE projects ADD COLUMN project_type_id INT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE projects ADD COLUMN booth_countdown INT NOT NULL DEFAULT 0;
ALTER TABLE projects ADD FOREIGN KEY (project_type_id) REFERENCES project_types(id)
