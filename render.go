package imagetpl

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // register GIF decoder
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"math"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// RenderJPEG composites the background image with the configured text elements
// and encodes the result as a JPEG. values is keyed by each element's Field;
// an empty value skips that element. quality is passed straight through to
// jpeg.Encode (use 95 for a good default). The background may be any format
// the standard library can decode (JPEG, PNG, GIF); the output is always JPEG.
func RenderJPEG(background []byte, elements []TemplateElement, values map[string]string, quality int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(background))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, src, image.Point{}, draw.Src)

	for _, el := range elements {
		el = resolveGeometry(el, bounds.Dx(), bounds.Dy())
		text := values[el.Field]
		if text == "" {
			continue
		}
		if err := drawTextElement(canvas, el, text); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resolveGeometry(el TemplateElement, width, height int) TemplateElement {
	if el.RelativePosition {
		el.X = int(math.Round(el.XRatio * float64(width)))
		el.Y = int(math.Round(el.YRatio * float64(height)))
	}
	if el.RelativeFontSize {
		el.FontSize = math.Max(1, el.FontSizeRatio*float64(height))
	}
	if el.RelativeMaxWidth {
		el.MaxWidth = int(math.Round(el.MaxWidthRatio * float64(width)))
	}
	return el
}

func drawTextElement(dst draw.Image, el TemplateElement, text string) error {
	size := el.FontSize
	if size <= 0 {
		size = defaultFontSize
	}
	face, err := faceForSize(el.Font, size)
	if err != nil {
		return err
	}
	drawText(dst, face, text, el.X, el.Y, parseColor(el.Color), el.Align, el.MaxWidth)
	return nil
}

// drawText renders text (with optional word-wrap and alignment) onto dst.
//
// X is an alignment anchor, not a box corner: for AlignLeft it is the left edge
// of each line, for AlignCenter the horizontal centre of each line, and for
// AlignRight the right edge. MaxWidth only controls word-wrapping (the max line
// width); it does not shift the anchor. Y is the top of the text.
func drawText(dst draw.Image, face font.Face, text string, x, y int, col color.Color, align string, maxWidth int) {
	m := face.Metrics()
	ascent := m.Ascent.Ceil()
	lineHeight := m.Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = ascent + m.Descent.Ceil()
	}

	for i, line := range wrapText(face, text, maxWidth) {
		lineWidth := font.MeasureString(face, line).Ceil()
		dx := x
		switch align {
		case AlignCenter:
			dx = x - lineWidth/2
		case AlignRight:
			dx = x - lineWidth
		}

		dr := &font.Drawer{
			Dst:  dst,
			Src:  image.NewUniform(col),
			Face: face,
			Dot:  fixed.P(dx, y+ascent+i*lineHeight),
		}
		dr.DrawString(line)
	}
}

// wrapText splits text into rendered lines. Explicit "\n" newlines are always
// honoured; when maxWidth > 0 words are additionally wrapped to fit it.
func wrapText(face font.Face, text string, maxWidth int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		if maxWidth <= 0 {
			lines = append(lines, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, w := range words[1:] {
			candidate := current + " " + w
			if font.MeasureString(face, candidate).Ceil() <= maxWidth {
				current = candidate
			} else {
				lines = append(lines, current)
				current = w
			}
		}
		lines = append(lines, current)
	}
	return lines
}

// parseColor converts a "#RRGGBB" string (with or without the leading '#') to
// an opaque colour, defaulting to white on any parse failure.
func parseColor(s string) color.Color {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.White
	}
	r, err1 := strconv.ParseUint(s[0:2], 16, 8)
	g, err2 := strconv.ParseUint(s[2:4], 16, 8)
	b, err3 := strconv.ParseUint(s[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return color.White
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

// --- font cache -------------------------------------------------------------

var (
	fontMu        sync.Mutex
	parsedFonts   = make(map[string]*opentype.Font)
	fontParseErrs = make(map[string]error)
)

func fontBytes(id string) []byte {
	switch id {
	case FontBold:
		return gobold.TTF
	case FontItalic:
		return goitalic.TTF
	case FontBoldItalic:
		return gobolditalic.TTF
	case FontMono:
		return gomono.TTF
	case FontRegular:
		return goregular.TTF
	default:
		return goregular.TTF
	}
}

func parsedFontValue(id string) (*opentype.Font, error) {
	if !isBuiltInFont(id) {
		id = FontRegular
	}

	fontMu.Lock()
	defer fontMu.Unlock()
	if parsed, ok := parsedFonts[id]; ok {
		return parsed, nil
	}
	if err, ok := fontParseErrs[id]; ok {
		return nil, err
	}
	parsed, err := opentype.Parse(fontBytes(id))
	if err != nil {
		fontParseErrs[id] = err
		return nil, err
	}
	parsedFonts[id] = parsed
	return parsed, nil
}

func faceForSize(id string, size float64) (font.Face, error) {
	parsed, err := parsedFontValue(id)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}
