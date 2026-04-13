package imaging

// OrientationFromCrop determines if the crop produces a landscape or portrait image.
// Returns "landscape" (1) or "portrait" (2) matching the overlay_orientations lookup table.
func OrientationFromTransform(t *TransformParams, imgWidth, imgHeight int) uint {
	if t == nil {
		if imgWidth >= imgHeight {
			return 1 // landscape
		}
		return 2 // portrait
	}

	// After rotation, effective dimensions may swap
	ew := float64(imgWidth)
	eh := float64(imgHeight)
	rot := int(t.Rotation) % 360
	if rot == 90 || rot == 270 {
		ew, eh = eh, ew
	}

	// Apply crop to get final dimensions
	cropW := t.CropWidth * ew
	cropH := t.CropHeight * eh

	if cropW >= cropH {
		return 1 // landscape
	}
	return 2 // portrait
}
