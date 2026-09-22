package imagetpl

// Alignment values for a text element. They control both the HTML preview
// (via a CSS class) and the server render (via line measurement).
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
)

// defaultFontSize is used when a text element does not set an explicit size.
// It is expressed in points, which at the renderer's 72 DPI equals pixels, so
// the HTML preview and the exported JPEG agree.
const defaultFontSize = 32.0

const defaultTextColor = "#ffffff"

// TemplateElement describes a single dynamic text layer drawn over the
// background template.
//
//   - Field is the form field name the end user fills in (e.g. "company_name").
//     It is also the key into the values map passed to RenderJPEG.
//   - Label is the human-readable label shown above the form input.
//   - X/Y is the top-left origin of the text box, in background pixels.
//   - FontSize is the point size (== px at 72 DPI).
//   - Color is a "#RRGGBB" hex colour (default "#ffffff").
//   - Align is one of AlignLeft / AlignCenter / AlignRight.
//   - Font is one of the built-in font IDs (default FontRegular).
//   - MaxWidth, when > 0, word-wraps the text to that pixel width; the
//     alignment is then measured against the full MaxWidth box.
type TemplateElement struct {
	Field    string
	Label    string
	X        int
	Y        int
	FontSize float64
	Color    string
	Align    string
	Font     string
	MaxWidth int
}
