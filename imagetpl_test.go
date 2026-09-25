package imagetpl

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleUploadAcceptsImageAndReplacesBackground(t *testing.T) {
	page := NewImageTemplatePage("Promo", "/promo")
	original := encodedImage(t, "jpeg", 20, 12, color.RGBA{R: 20, G: 30, B: 40, A: 255})
	page.BackgroundBytes(original)
	replacement := encodedImage(t, "png", 40, 24, color.RGBA{R: 80, G: 90, B: 100, A: 255})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("background", "replacement.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(replacement); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/promo/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	page.handleUpload(response, req)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d; body=%q", response.Code, http.StatusSeeOther, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != "/promo" {
		t.Fatalf("redirect = %q, want /promo", got)
	}

	snapshot := page.snapshot()
	if !bytes.Equal(snapshot.background, replacement) {
		t.Fatal("validated upload did not replace the page background")
	}
	if snapshot.bgWidth != 40 || snapshot.bgHeight != 24 {
		t.Fatalf("background dimensions = %dx%d, want 40x24", snapshot.bgWidth, snapshot.bgHeight)
	}

	backgroundResponse := httptest.NewRecorder()
	page.handleBackground(backgroundResponse, httptest.NewRequest(http.MethodGet, "/promo/background", nil))
	if backgroundResponse.Code != http.StatusOK {
		t.Fatalf("background status = %d, want %d", backgroundResponse.Code, http.StatusOK)
	}
	if got := backgroundResponse.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("background content type = %q, want image/png", got)
	}
	if !bytes.Equal(backgroundResponse.Body.Bytes(), replacement) {
		t.Fatal("background endpoint did not serve the accepted upload")
	}
}

func TestHandleUploadRejectsInvalidImageWithoutChangingBackground(t *testing.T) {
	page := NewImageTemplatePage("Promo", "/promo")
	original := encodedImage(t, "png", 16, 10, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	page.BackgroundBytes(original)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("background", "not-an-image.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	_, _ = part.Write([]byte("this is not an image"))
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/promo/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	page.handleUpload(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid upload status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !bytes.Equal(page.snapshot().background, original) {
		t.Fatal("invalid upload changed the existing background")
	}
}

func TestUploadedBackgroundSurvivesPageRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background")
	store := FileBackgroundStore{Path: path}
	page := NewImageTemplatePage("Promo", "/promo")
	if err := page.UseBackgroundStore(store); err != nil {
		t.Fatalf("configure store: %v", err)
	}
	background := encodedImage(t, "png", 40, 24, color.RGBA{R: 80, G: 90, B: 100, A: 255})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("background", "background.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(background); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/promo/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	page.handleUpload(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d: %s", response.Code, response.Body.String())
	}

	restarted := NewImageTemplatePage("Promo", "/promo")
	if err := restarted.UseBackgroundStore(store); err != nil {
		t.Fatalf("restore background: %v", err)
	}
	if !bytes.Equal(restarted.snapshot().background, background) {
		t.Fatal("restart did not restore the accepted image")
	}
	backgroundResponse := httptest.NewRecorder()
	restarted.handleBackground(backgroundResponse, httptest.NewRequest(http.MethodGet, "/promo/background", nil))
	if !bytes.Equal(backgroundResponse.Body.Bytes(), background) {
		t.Fatal("restored background endpoint did not serve the original bytes")
	}
}

func TestRequestValuesAreLimitedToConfiguredFields(t *testing.T) {
	page := NewImageTemplatePage("Promo", "/promo").
		ValuesFn(func(r *http.Request) map[string]string {
			return map[string]string{"message": r.URL.Query().Get("message"), "ignored": "private"}
		}).
		Text("message", "Message").End()
	request := httptest.NewRequest(http.MethodGet, "/promo?message=Unavailable", nil)
	data := page.buildPageData(request)
	if data.FieldDefaults["message"] != "Unavailable" {
		t.Fatalf("default message = %q", data.FieldDefaults["message"])
	}
	if _, exists := data.FieldDefaults["ignored"]; exists {
		t.Fatal("unconfigured field was exposed to the editor")
	}
}

func TestRelativeTextGeometryTracksBackgroundDimensions(t *testing.T) {
	page := NewImageTemplatePage("Promo", "/promo").
		Text("message", "Message").
		PositionRelative(0.5, 0.4).
		FontSizeRelative(0.06).
		MaxWidthRelative(0.8).
		End()
	element := page.snapshot().elements[0]
	first := resolveGeometry(element, 1920, 1080)
	second := resolveGeometry(element, 1280, 720)
	if first.X != 960 || first.Y != 432 || first.MaxWidth != 1536 || math.Abs(first.FontSize-64.8) > 0.001 {
		t.Fatalf("full HD geometry = %+v", first)
	}
	if second.X != 640 || second.Y != 288 || second.MaxWidth != 1024 || math.Abs(second.FontSize-43.2) > 0.001 {
		t.Fatalf("HD geometry = %+v", second)
	}
}

func TestPublishPassesRenderedJPEGAndSubmittedValuesToHook(t *testing.T) {
	background := encodedImage(t, "png", 180, 80, color.RGBA{R: 30, G: 50, B: 70, A: 255})
	called := false
	page := NewImageTemplatePage("Promo", "/promo").
		BackgroundBytes(background).
		Text("message", "Message").Position(90, 12).Align(AlignCenter).End().
		ExportHook(func(_ *http.Request, result ExportResult) error {
			called = true
			if result.Values["message"] != "Unavailable" || result.Width != 180 || result.Height != 80 {
				t.Errorf("hook result = %+v", result)
			}
			if _, err := jpeg.Decode(bytes.NewReader(result.Data)); err != nil {
				t.Errorf("hook did not receive a JPEG: %v", err)
			}
			return nil
		})
	form := url.Values{"message": {"Unavailable"}}
	request := httptest.NewRequest(http.MethodPost, "/promo/publish", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	page.handlePublish(response, request)
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("publish called = %v, status = %d, body = %q", called, response.Code, response.Body.String())
	}
}

func TestPublishAcceptsMultipartEditorValues(t *testing.T) {
	background := encodedImage(t, "png", 180, 80, color.RGBA{R: 30, G: 50, B: 70, A: 255})
	var published string
	page := NewImageTemplatePage("Promo", "/promo").
		BackgroundBytes(background).
		Text("message", "Message").Position(90, 12).Align(AlignCenter).End().
		ExportHook(func(_ *http.Request, result ExportResult) error {
			published = result.Values["message"]
			return nil
		})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("message", "Unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/promo/publish", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	page.handlePublish(response, request)
	if response.Code != http.StatusNoContent || published != "Unavailable" {
		t.Fatalf("publish status = %d, message = %q", response.Code, published)
	}
}

func TestResolveSubmissionBoundsEditableValues(t *testing.T) {
	page := NewImageTemplatePage("Promo", "/promo").
		FontOptions(FontOption{ID: FontRegular, Label: "Regular"}, FontOption{ID: FontBold, Label: "Bold"}).
		Text("headline", "Headline").
		Font(FontBold).
		Color("#ABCDEF").
		End()

	longText := strings.Repeat("x", maxTextLength+10)
	form := url.Values{
		"headline":                 []string{longText},
		fontFieldName("headline"):  []string{"unknown"},
		colorFieldName("headline"): []string{"javascript:alert(1)"},
	}
	req := httptest.NewRequest(http.MethodPost, "/promo/export", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	elements, values := page.resolveSubmission(req, page.snapshot())
	if got := len([]rune(values["headline"])); got != maxTextLength {
		t.Fatalf("headline length = %d, want %d", got, maxTextLength)
	}
	if elements[0].Font != FontBold {
		t.Fatalf("invalid submitted font = %q, want fallback %q", elements[0].Font, FontBold)
	}
	if elements[0].Color != "#abcdef" {
		t.Fatalf("invalid submitted colour = %q, want #abcdef", elements[0].Color)
	}
}

func TestRenderJPEGUsesConfiguredFontAndColour(t *testing.T) {
	background := encodedImage(t, "png", 180, 80, color.RGBA{R: 240, G: 240, B: 240, A: 255})
	elements := []TemplateElement{{
		Field:    "headline",
		X:        90,
		Y:        10,
		FontSize: 32,
		Font:     FontBold,
		Color:    "#ff0000",
		Align:    AlignCenter,
	}}

	data, err := RenderJPEG(background, elements, map[string]string{"headline": "Preview"}, 95)
	if err != nil {
		t.Fatalf("render JPEG: %v", err)
	}
	rendered, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode rendered JPEG: %v", err)
	}

	redPixels := 0
	for y := rendered.Bounds().Min.Y; y < rendered.Bounds().Max.Y; y++ {
		for x := rendered.Bounds().Min.X; x < rendered.Bounds().Max.X; x++ {
			r, g, b, _ := rendered.At(x, y).RGBA()
			if r > 0xc000 && g < 0x7000 && b < 0x7000 {
				redPixels++
			}
		}
	}
	if redPixels == 0 {
		t.Fatal("rendered JPEG contains no pixels matching the requested text colour")
	}
}

func TestHandleExportCompositesMessageAndMultilineDescription(t *testing.T) {
	background := encodedImage(t, "png", 400, 240, color.RGBA{R: 24, G: 24, B: 24, A: 255})
	page := NewImageTemplatePage("Promo", "/promo").
		BackgroundBytes(background).
		Text("message", "Message").Position(200, 32).FontSize(38).Color("#ff0000").Align(AlignCenter).
		Text("description", "Description").Multiline().Position(200, 126).FontSize(27).Color("#00aaff").Align(AlignCenter).End()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for field, value := range map[string]string{
		"message":     "Unavailable",
		"description": "First line\nSecond line",
	} {
		if err := writer.WriteField(field, value); err != nil {
			t.Fatalf("write %s: %v", field, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/promo/export", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	page.handleExport(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("export status = %d, type = %q, body = %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	rendered, err := jpeg.Decode(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode export: %v", err)
	}
	var redMessage, blueDescription int
	for y := 0; y < 240; y++ {
		for x := 0; x < 400; x++ {
			r, g, b, _ := rendered.At(x, y).RGBA()
			if y < 110 && r > 0xb000 && g < 0x6000 && b < 0x6000 {
				redMessage++
			}
			if y >= 126 && b > 0xa000 && r < 0x6000 && g > 0x5000 {
				blueDescription++
			}
		}
	}
	if redMessage == 0 || blueDescription == 0 {
		t.Fatalf("export text pixels: message=%d, description=%d", redMessage, blueDescription)
	}
}

func TestSafeDownloadFilenamePreventsHeaderInjection(t *testing.T) {
	tests := map[string]string{
		"../../promo":            "promo.jpg",
		"promo\r\nX-Injected: 1": "promoX-Injected: 1.jpg",
		"":                       "template.jpg",
		"finished.JPEG":          "finished.JPEG",
	}
	for input, want := range tests {
		if got := safeDownloadFilename(input); got != want {
			t.Errorf("safeDownloadFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func encodedImage(t *testing.T, format string, width, height int, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fill)
		}
	}

	var output bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&output, img, &jpeg.Options{Quality: 95})
	case "png":
		err = png.Encode(&output, img)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s image: %v", format, err)
	}
	return output.Bytes()
}
