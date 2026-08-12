-- Where taken_at came from, so the UI can qualify a date the camera never
-- recorded. One of 'exif', 'iptc' (a desktop editor's DateCreated) or 'file'
-- (the uploaded file's modification time, a last resort).
--
-- NULL alongside a non-NULL taken_at means a row written before this column
-- existed, when EXIF was the only source there was — such rows read as 'exif'.
ALTER TABLE photos ADD COLUMN taken_at_source TEXT;
