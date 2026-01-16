package quarkgl

// Target is a minimal pixel target for software rendering.
//
// Implementations should clip out-of-bounds coordinates.
type Target interface {
	Size() (w, h int)
	SetPixel(x, y int, c Color)
	Clear(c Color)
}

// RenderMode selects the rasterization mode.
type RenderMode uint8

const (
	RenderWireframe RenderMode = iota
	RenderSolidFlat
	RenderSolidVertexColor
)
