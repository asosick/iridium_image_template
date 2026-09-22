package imagetpl

import "strings"

const (
	FontRegular    = "regular"
	FontBold       = "bold"
	FontItalic     = "italic"
	FontBoldItalic = "bold-italic"
	FontMono       = "mono"
)

// FontOption is one of the embedded fonts available to the preview and JPEG
// renderer. The browser preview uses a close system-font equivalent; the
// server always renders with the matching embedded Go font.
type FontOption struct {
	ID    string
	Label string
}

var builtInFontOptions = []FontOption{
	{ID: FontRegular, Label: "Regular"},
	{ID: FontBold, Label: "Bold"},
	{ID: FontItalic, Label: "Italic"},
	{ID: FontBoldItalic, Label: "Bold italic"},
	{ID: FontMono, Label: "Monospace"},
}

// DefaultFontOptions returns the standard choices shown by a new page.
func DefaultFontOptions() []FontOption {
	return cloneFontOptions(builtInFontOptions)
}

func cloneFontOptions(options []FontOption) []FontOption {
	return append([]FontOption(nil), options...)
}

func normaliseFontOptions(options []FontOption) []FontOption {
	if len(options) == 0 {
		return DefaultFontOptions()
	}

	allowed := make(map[string]FontOption, len(builtInFontOptions))
	for _, option := range builtInFontOptions {
		allowed[option.ID] = option
	}

	seen := make(map[string]struct{}, len(options))
	result := make([]FontOption, 0, len(options))
	for _, option := range options {
		id := strings.TrimSpace(option.ID)
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if strings.TrimSpace(option.Label) == "" {
			option.Label = allowed[id].Label
		}
		option.ID = id
		seen[id] = struct{}{}
		result = append(result, option)
	}
	if len(result) == 0 {
		return DefaultFontOptions()
	}
	return result
}

func isBuiltInFont(id string) bool {
	for _, option := range builtInFontOptions {
		if option.ID == id {
			return true
		}
	}
	return false
}
