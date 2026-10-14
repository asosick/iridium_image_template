package imagetpl

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/iridiumgo/iridium/core/context/ctxPanel"
	"github.com/iridiumgo/iridium/core/logger"
	"github.com/iridiumgo/iridium/network/wrapper"
)

const (
	routeUpload     = "/upload"
	routeBackground = "/background"
	routeExport     = "/export"
	routePublish    = "/publish"

	maxUploadBytes        = 30 << 20
	maxMultipartBodyBytes = maxUploadBytes + 1<<20
	maxUploadMemory       = 2 << 20
	maxEditorBodyBytes    = 1 << 20
	maxEditorMemory       = 64 << 10
)

var (
	errUnsupportedBackground = errors.New("unsupported background image format")
	errBackgroundDimensions  = errors.New("background image dimensions exceed the limit")
)

func (p *ImageTemplatePage) registerPluginRoutes(mux wrapper.IMux) {
	mux.HandleFunc(routeUpload, p.handleUpload)
	mux.HandleFunc(routeBackground, p.handleBackground)
	mux.HandleFunc(routeExport, p.handleExport)
	mux.HandleFunc(routePublish, p.handlePublish)
}

// handleUpload accepts one background image and redirects through Iridium's
// hard-reload path so a normal form and an HTMX request behave consistently.
func (p *ImageTemplatePage) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength > maxMultipartBodyBytes {
		p.uploadError(w, r, http.StatusRequestEntityTooLarge, "The background image is too large.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBodyBytes)
	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		p.uploadError(w, r, http.StatusBadRequest, "The background image could not be read.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("background")
	if err != nil {
		p.uploadError(w, r, http.StatusBadRequest, "Choose a background image first.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		p.uploadError(w, r, http.StatusBadRequest, "The background image could not be read.")
		return
	}
	if len(data) > maxUploadBytes {
		p.uploadError(w, r, http.StatusRequestEntityTooLarge, "The background image is too large.")
		return
	}

	width, height, err := validateBackground(data)
	if err != nil {
		p.uploadError(w, r, http.StatusBadRequest, "Use a valid JPEG, PNG, or GIF image with reasonable dimensions.")
		return
	}
	if err := p.saveBackground(r.Context(), data, width, height); err != nil {
		logger.Error("[imagetpl] saving background failed: %v", err)
		p.uploadError(w, r, http.StatusInternalServerError, "The background could not be saved.")
		return
	}
	p.redirectToPage(w, r)
}

func validateBackground(data []byte) (width, height int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return 0, 0, errUnsupportedBackground
	}
	if !validBackgroundDimensions(cfg.Width, cfg.Height) {
		return 0, 0, errBackgroundDimensions
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	bounds := decoded.Bounds()
	if !validBackgroundDimensions(bounds.Dx(), bounds.Dy()) {
		return 0, 0, errBackgroundDimensions
	}
	return bounds.Dx(), bounds.Dy(), nil
}

func (p *ImageTemplatePage) uploadError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if r.Header.Get("HX-Request") == "true" {
		ctx := ctxPanel.NewPanelPageContext(r, w)
		ctx.AppendHXTrigger("notify", map[string]string{
			"type":        "error",
			"message":     "Upload failed",
			"description": message,
		})
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, message, status)
}

func (p *ImageTemplatePage) redirectToPage(w http.ResponseWriter, r *http.Request) {
	ctxPanel.NewPanelPageContext(r, w).HardRedirect(p.SlugStr)
}

func (p *ImageTemplatePage) handleBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	p.mu.RLock()
	background := append([]byte(nil), p.background...)
	p.mu.RUnlock()
	if len(background) == 0 {
		http.Error(w, "no background uploaded", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", http.DetectContentType(background))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(background)))
		return
	}
	_, _ = w.Write(background)
}

func (p *ImageTemplatePage) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := p.exportDenied(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if !parseEditorForm(w, r) {
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	snapshot := p.snapshot()
	if len(snapshot.background) == 0 {
		http.Error(w, "no background template uploaded", http.StatusBadRequest)
		return
	}

	elements, values := p.resolveSubmission(r, snapshot)
	out, err := RenderJPEG(snapshot.background, elements, values, 95)
	if err != nil {
		logger.Error("[imagetpl] export render failed: %v", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}

	filename := safeDownloadFilename(snapshot.filename)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
}

func (p *ImageTemplatePage) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := p.exportDenied(r); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if !parseEditorForm(w, r) {
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	snapshot := p.snapshot()
	if len(snapshot.background) == 0 {
		http.Error(w, "no background template uploaded", http.StatusBadRequest)
		return
	}
	if snapshot.exportHook == nil {
		http.Error(w, "no export hook configured", http.StatusBadRequest)
		return
	}

	elements, values := p.resolveSubmission(r, snapshot)
	out, err := RenderJPEG(snapshot.background, elements, values, 95)
	if err != nil {
		logger.Error("[imagetpl] publish render failed: %v", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}

	result := ExportResult{
		Data:     out,
		Filename: safeDownloadFilename(snapshot.filename),
		Width:    snapshot.bgWidth,
		Height:   snapshot.bgHeight,
		Values:   values,
	}
	if err := snapshot.exportHook(r, result); err != nil {
		logger.Error("[imagetpl] export hook failed: %v", err)
		ctxPanel.NewPanelPageContext(r, w).AppendHXTrigger("notify", map[string]string{
			"type":        "error",
			"message":     "Publish failed",
			"description": "The server could not process the image.",
		})
		w.WriteHeader(http.StatusOK)
		return
	}

	ctxPanel.NewPanelPageContext(r, w).AppendHXTrigger("notify", map[string]string{
		"type":        "success",
		"message":     "Published",
		"description": "The image was processed successfully.",
	})
	w.WriteHeader(http.StatusNoContent)
}

func parseEditorForm(w http.ResponseWriter, r *http.Request) bool {
	// FormData downloads arrive as multipart; ParseForm alone leaves their fields empty.
	if r.ContentLength > maxEditorBodyBytes {
		http.Error(w, "form too large", http.StatusRequestEntityTooLarge)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEditorBodyBytes)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil {
		switch mediaType {
		case "multipart/form-data":
			err = r.ParseMultipartForm(maxEditorMemory)
		case "application/x-www-form-urlencoded":
			err = r.ParseForm()
		default:
			err = errors.New("unsupported form type")
		}
	}
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "form too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "bad form", http.StatusBadRequest)
	}
	return false
}

func (p *ImageTemplatePage) resolveSubmission(r *http.Request, snapshot pageSnapshot) ([]TemplateElement, map[string]string) {
	elements := append([]TemplateElement(nil), snapshot.elements...)
	values := make(map[string]string, len(elements))
	for i := range elements {
		element := &elements[i]
		values[element.Field] = limitText(r.PostFormValue(element.Field))
		element.Font = submittedFont(r.PostFormValue(fontFieldName(element.Field)), element.Font, snapshot.fonts)
		element.Color = normaliseColor(r.PostFormValue(colorFieldName(element.Field)), normaliseColor(element.Color, defaultTextColor))
	}
	return elements, values
}

func submittedFont(requested, fallback string, options []FontOption) string {
	for _, option := range options {
		if requested == option.ID {
			return requested
		}
	}
	for _, option := range options {
		if fallback == option.ID {
			return fallback
		}
	}
	if len(options) > 0 {
		return options[0].ID
	}
	return FontRegular
}

func safeDownloadFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = path.Base(strings.ReplaceAll(filename, "\\", "/"))
	filename = strings.Trim(filename, ". ")
	filename = strings.ReplaceAll(filename, "\r", "")
	filename = strings.ReplaceAll(filename, "\n", "")
	if filename == "" || filename == "." || filename == ".." {
		return "template.jpg"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".jpg") && !strings.HasSuffix(strings.ToLower(filename), ".jpeg") {
		filename += ".jpg"
	}
	return filename
}
