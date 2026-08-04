-- Capture metadata read from EXIF at upload time. taken_at holds the camera's
-- wall-clock reading. A NULL means the file carried no usable timestamp, in
-- which case consumers fall back to created_at (the upload time).
ALTER TABLE photos ADD COLUMN taken_at DATETIME NULL;
ALTER TABLE photos ADD COLUMN camera_make VARCHAR(128) NULL;
ALTER TABLE photos ADD COLUMN camera_model VARCHAR(128) NULL;

-- Text overlays can now derive their content from the photo instead of
-- carrying literal text. The 'static' default preserves existing behavior
-- for every row written before this migration.
ALTER TABLE text_overlays ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'static';
ALTER TABLE text_overlays ADD COLUMN date_format VARCHAR(32) NULL
