# Iridium Image Template

A standalone plugin for [Iridium](https://github.com/iridiumgo/iridium) that
turns a template image into a fill-in-the-blanks image generator.

Upload (or configure) a background image — say a promotional banner with the
company logo already baked into the corner — define a few dynamic **text
elements** on top of it, and let end users type the text and export a flattened
JPEG. The exported image is rendered server-side with Go's standard `image`
stack, so the result is authoritative and high-resolution (the browser preview
is only for convenience).

```
┌──────────────────────────────────────┐
│                              [LOGO]  │
│                                      │
│          {{ company_name }}          │
│                                      │
│       {{ promotional_text }}         │
│                                      │
└──────────────────────────────────────┘
```

## What it does

- **Upload a background image** — JPEG, PNG or GIF — or set a default from a
  file / bytes.
- **Edit text fields** with an instant live preview (Alpine `x-model` → `x-text`).
- **Adjust each layer** with a font selector and colour picker; the selected
  styles are used again when the image is exported.
- **Restore the last draft** automatically in the same browser, including text,
  fonts, and colours. This can be disabled per page.
- **Download** the flattened JPEG.
- **Publish** the flattened JPEG through a server-side hook — FTP it, push it to
  S3, write it to disk, POST it to a media service, etc.

The uploaded background is never modified. Each export composites a fresh JPEG
from `background + text layers` and streams it back on demand.

## Run the demo

```bash
go run ./test
```

Then open <http://127.0.0.1:8099/admin/promo>. The demo generates a placeholder
banner in code, so there is nothing to configure. Its "Publish" button writes
the rendered JPEG to `./exports/` as a stand-in for a real FTP/S3 integration.

> The test app's panel applies `middleware.Spa` — **without it the page renders
> as a bare fragment** (no `<head>`, no sidebar, no CSS). See
> [Requirements](#requirements).

## Usage

```go
package main

import (
    imagetpl "github.com/asosick/iridium_image_template"
)

func promoPage() *imagetpl.ImageTemplatePage {
    return imagetpl.NewImageTemplatePage("Promo Banner", "promo").
        Background("assets/promo.jpg").           // or BackgroundBytes(...)
        Filename("promo.jpg").
        Text("company_name", "Company Name").
            Position(600, 250).
            FontSize(58).
            Font(imagetpl.FontBold).
            Align(imagetpl.AlignCenter).
            MaxWidth(980).
        Text("promo_text", "Promotional Message").
            Position(600, 345).
            FontSize(30).
            Color("#ffffff").
            Align(imagetpl.AlignCenter).
            MaxWidth(820).
        End()
}
```

Register the page with `PanelItems`. The panel should include request recovery
and body-size middleware, plus `middleware.Spa` so the editor loads inside the
Iridium app shell.

### Server-side processing (`ExportHook`)

Register a hook to act on the rendered image when the user clicks the publish
button:

```go
p.ExportHook(func(r *http.Request, result imagetpl.ExportResult) error {
    // result.Data      []byte            — the flattened JPEG
    // result.Filename  string
    // result.Width/Height int
    // result.Values    map[string]string — submitted text keyed by Field
    return ftp.Upload("banners/"+result.Filename, result.Data)
})
```

`ExportHook` is set → the editor shows an extra publish button (label via
`PublishLabel`, default "Publish"). The hook runs server-side on `/publish` and
responds with a success/error notification; the download button is unaffected.

### Text element options

| Method | Description |
|---|---|
| `Position(x, y)` | Top-left origin in background pixels. |
| `FontSize(pt)` | Point size (== px at the renderer's 72 DPI). |
| `Color("#RRGGBB")` | Text colour (default `#ffffff`). |
| `Align(...)` | `AlignLeft` / `AlignCenter` / `AlignRight`. |
| `Font(...)` | Default font: `FontRegular`, `FontBold`, `FontItalic`, `FontBoldItalic`, or `FontMono`. |
| `MaxWidth(px)` | Word-wrap width; alignment is measured against it. |

`Text(field, label)` starts a new element (committing the previous one);
`End()` finishes the last element and returns the page.

The editor exposes the configured font choices and a colour picker for every
layer. Use `FontOptions(...)` to reduce the list to a subset of the embedded
fonts. `RememberValues(false)` disables browser-local draft restoration. Drafts
are namespaced by panel and page, and contain only the editable text, font, and
colour values; they are not a server-side source of truth.

## Architecture

```
ImageTemplatePage (panel.CustomPanelPage)
├── background []byte            // uploaded/default image, shared by this page process-wide
├── elements   []TemplateElement // the dynamic text layers
├── fonts      []FontOption      // allowed embedded fonts
├── exportHook ExportHook        // optional server-side processing
│
├── GET  /<page>            → views.Editor    (form + live preview)
├── POST /<page>/upload     → store background (multipart "background")
├── GET  /<page>/background → stream the current background image
├── POST /<page>/export     → render + download (Content-Disposition)
└── POST /<page>/publish    → render + call ExportHook + notify
```

### Rendering (`render.go`)

`RenderJPEG` decodes the background, draws each text layer onto an RGBA canvas,
and re-encodes as JPEG. Text is rendered with the selected embedded Go font
through `golang.org/x/image/font/opentype`, with word-wrap and alignment.
`FontSize` is in points at 72 DPI, so 1pt == 1px and the preview and export
agree.

### Preview (`views/page.templ` + `static/imagetpl.css`)

The background `<img>` is rendered with `width: 100%; height: auto` so it always
fills the preview width and keeps its aspect ratio. Overlay anchors and font
sizes scale with the background using percentages and container-query units
(`cqw`), without JavaScript measuring. The browser uses system fonts as close
matches for the embedded export fonts, so glyph widths and line wrapping can
differ slightly in the downloaded image. The preview is wrapped in an Iridium
`ir-card` for a themed border. The `imagetpl` Alpine component owns the
field/style values for live updates and persists a versioned, page-scoped draft
in `localStorage`. The clear action removes that draft and restores the page
defaults. Browser storage failures do not prevent editing or exporting.

### Inputs (`views/input.go`)

The text fields render through Iridium's own concrete input component
(`bfield.TextInputBlock` + `field.BaseFieldConcrete`, resolved by
`view/blocks/fields.NewInput`), so they pick up the host theme's `ir-input`,
`ir-field-wrapper`, label and dark-mode styling for free. Each input is bound to
the shared Alpine `fields` scope with `x-model`.

## Requirements / gotchas

- **Panel middleware.** Your panel must apply `middleware.Spa` (usually alongside
  `middleware.Compression` and `middleware.ErrorPages`) or the page renders as a
  fragment without the app shell and this plugin's CSS.
- **CSP Alpine.** Iridium ships the CSP build of Alpine, so `x-data` must
  reference a registered component name — never an inline object. This plugin
  registers `Alpine.data("imagetpl", ...)` in `static/imagetpl.js`.
- **Background storage is in-memory.** The uploaded background lives on the page
  struct for the process lifetime; it is shared by all users of that page and is
  not durable across restarts. For durable storage, persist accepted bytes at
  the upload boundary and load them through `BackgroundBytes` during page
  construction. Uploads are limited to 32 MiB and 40 megapixels and are decoded
  before replacing the current background.
- **Draft storage is browser-local.** Remembered values are deliberately kept in
  `localStorage` so a user can return to the same page without introducing a
  server-side draft store. Do not put secrets or sensitive material in template
  fields.
- **Build requirements.** Use Go 1.27 or newer and a compatible Iridium release. The
  demo keys are for local development only; generate your own keys for an app
  and do not reuse the demo values.

## Layout

```
doc.go        package docs + usage
element.go    TemplateElement type + alignment constants
page.go       ImageTemplatePage builder, GetComponent, RegisterRoutes
export.go     ExportResult / ExportHook + Filename / ExportHook / PublishLabel
render.go     JPEG compositing + text drawing (font/wrap/align/colour)
routes.go     upload / background / export / publish handlers
embed.go      CSS + JS asset registration
views/        templ views + concrete Iridium inputs (page.templ, input.go)
static/       imagetpl.css + imagetpl.js (Alpine component)
test/         single-page runnable demo (go run ./test)
```

## License

MIT
