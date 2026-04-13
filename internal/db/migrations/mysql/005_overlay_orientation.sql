CREATE TABLE overlay_orientations (
    id   INT UNSIGNED PRIMARY KEY,
    name VARCHAR(20) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO overlay_orientations (id, name) VALUES (1, 'landscape');
INSERT INTO overlay_orientations (id, name) VALUES (2, 'portrait');

ALTER TABLE overlays ADD COLUMN orientation_id INT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE text_overlays ADD COLUMN orientation_id INT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE overlays ADD FOREIGN KEY (orientation_id) REFERENCES overlay_orientations(id);
ALTER TABLE text_overlays ADD FOREIGN KEY (orientation_id) REFERENCES overlay_orientations(id)
