-- SHA-256 of the bytes as uploaded, used to spot a photo that is already in
-- the project so the guest can be warned before a second copy is registered.
-- NULL on rows written before this migration: those photos were never hashed,
-- so they simply never match.
ALTER TABLE photos ADD COLUMN content_hash CHAR(64) NULL;

-- Not unique: uploading the same photo twice stays allowed once the guest
-- confirms it. The index only has to make the lookup cheap.
CREATE INDEX idx_photos_project_hash ON photos(project_id, content_hash)
