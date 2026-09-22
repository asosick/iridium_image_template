package imagetpl

import (
	"strings"
)

const (
	maxBackgroundPixels int64 = 40_000_000
	maxTextLength             = 2000
	fontFieldPrefix           = "_imagetpl_font_"
	colorFieldPrefix          = "_imagetpl_color_"
)

func persistenceKey(route string) string {
	return "iridium:image-template:v2:" + route
}

func fontFieldName(field string) string {
	return fontFieldPrefix + field
}

func colorFieldName(field string) string {
	return colorFieldPrefix + field
}

func isHexColor(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 7 && value[0] == '#' {
		value = value[1:]
	}
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func normaliseColor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if !isHexColor(value) {
		return fallback
	}
	if value[0] != '#' {
		value = "#" + value
	}
	return strings.ToLower(value)
}

func limitText(value string) string {
	runes := []rune(value)
	if len(runes) <= maxTextLength {
		return value
	}
	return string(runes[:maxTextLength])
}

func validBackgroundDimensions(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	return int64(width) <= maxBackgroundPixels/int64(height)
}
