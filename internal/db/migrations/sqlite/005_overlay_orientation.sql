CREATE TABLE overlay_orientations (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

INSERT INTO overlay_orientations (id, name) VALUES (1, 'landscape');
INSERT INTO overlay_orientations (id, name) VALUES (2, 'portrait');

ALTER TABLE overlays ADD COLUMN orientation_id INTEGER NOT NULL DEFAULT 1 REFERENCES overlay_orientations(id);
ALTER TABLE text_overlays ADD COLUMN orientation_id INTEGER NOT NULL DEFAULT 1 REFERENCES overlay_orientations(id)
