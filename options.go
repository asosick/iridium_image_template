package imagetpl

// FontOptions controls the choices shown for every text layer in the editor.
// The options are limited to the embedded fonts returned by
// DefaultFontOptions so browser preview and server export remain consistent.
func (p *ImageTemplatePage) FontOptions(options ...FontOption) *ImageTemplatePage {
	p.mu.Lock()
	p.fonts = normaliseFontOptions(options)
	p.mu.Unlock()
	return p
}

// RememberValues keeps the last text, font, and colour choices in a
// page-scoped browser draft. It is enabled by default and can be disabled for
// pages that should always open blank.
func (p *ImageTemplatePage) RememberValues(enabled bool) *ImageTemplatePage {
	p.mu.Lock()
	p.remember = enabled
	p.mu.Unlock()
	return p
}
