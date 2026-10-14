package imagetpl

import "net/http"

// ExportResult is handed to an ExportHook after a successful render. It
// carries everything the hook needs to act on the image server-side.
type ExportResult struct {
	// Data is the flattened JPEG (background + text layers).
	Data []byte
	// Filename is the configured download filename (see Filename).
	Filename string
	// Width / Height are the background pixel dimensions (== the output size).
	Width  int
	Height int
	// Values are the submitted field values keyed by element Field.
	Values map[string]string
}

// ExportHook is an optional server-side callback invoked when the user presses
// the publish button. It receives the rendered JPEG so you can act on it —
// FTP it to a remote server, upload to S3, save to disk, email it, etc.
// Returning an error surfaces a failure notification and aborts the publish.
type ExportHook func(r *http.Request, result ExportResult) error

// ExportGate decides, per request, whether the finished image may leave the
// page: be downloaded (/export) or handed to the ExportHook (/publish). Return
// nil to allow it. A returned error's message is shown to the user:
//
//   - the editor shows it as a notice, and the Download button reports it
//     instead of sending a request;
//   - the server answers 403 with it, so a request sent anyway is refused.
//
// The editor, the live preview and the background upload are not affected, so
// a page can be tried before it is unlocked.
type ExportGate func(r *http.Request) error

// ExportGate registers the gate for downloading and publishing.
func (p *ImageTemplatePage) ExportGate(fn ExportGate) *ImageTemplatePage {
	p.mu.Lock()
	p.exportGate = fn
	p.mu.Unlock()
	return p
}

// exportDenied returns the gate's refusal for a request, or nil.
func (p *ImageTemplatePage) exportDenied(r *http.Request) error {
	p.mu.RLock()
	gate := p.exportGate
	p.mu.RUnlock()
	if gate == nil {
		return nil
	}
	return gate(r)
}

// Filename sets the download filename for the exported JPEG (used in the
// Content-Disposition header and passed to the ExportHook). Defaults to
// "template.jpg".
func (p *ImageTemplatePage) Filename(name string) *ImageTemplatePage {
	if name != "" {
		p.mu.Lock()
		p.filename = name
		p.mu.Unlock()
	}
	return p
}

// ExportHook registers the server-side processing hook. When set, the editor
// shows an additional publish button (see PublishLabel) that renders the JPEG
// and calls the hook instead of downloading it.
func (p *ImageTemplatePage) ExportHook(fn ExportHook) *ImageTemplatePage {
	if fn != nil {
		p.mu.Lock()
		p.exportHook = fn
		p.mu.Unlock()
	}
	return p
}

// PublishLabel sets the label of the publish button shown when an ExportHook
// is registered. Defaults to "Publish".
func (p *ImageTemplatePage) PublishLabel(label string) *ImageTemplatePage {
	if label != "" {
		p.mu.Lock()
		p.publishLabel = label
		p.mu.Unlock()
	}
	return p
}
