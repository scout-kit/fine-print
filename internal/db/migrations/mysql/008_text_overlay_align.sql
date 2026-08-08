-- Which edge of the text stays pinned to the overlay's stored x position.
-- Date overlays render a different string per photo, so a left-anchored date
-- on the right side of a print drifts and can run off the edge as the text
-- gets longer. Anchoring right keeps that edge fixed and grows leftward,
-- and center keeps the text balanced around x.
-- 'left' matches how every existing overlay was already positioned.
ALTER TABLE text_overlays ADD COLUMN text_align VARCHAR(10) NOT NULL DEFAULT 'left'
