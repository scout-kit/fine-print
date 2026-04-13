package imaging

// RenderOptions holds all parameters needed to produce a final print-ready image.
type RenderOptions struct {
	Transform    *TransformParams
	Color        *ColorParams
	Overlays     []OverlayParams
	TextOverlays []TextParams
}
