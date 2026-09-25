package imagetpl

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/iridiumgo/iridium/core/context/ctxPanel"
	"github.com/iridiumgo/iridium/core/context/ctxkeys"
	"github.com/iridiumgo/iridium/core/logger"
	"github.com/iridiumgo/iridium/core/page/panel"
	"github.com/iridiumgo/iridium/network/wrapper"

	"github.com/asosick/iridium_image_template/views"
)

// ImageTemplatePage is an Iridium panel page that lets an admin upload a
// background image and an end user fill in the dynamic text on top of it, then
// export a flattened JPEG. Register it with your panel like any other page:
//
//	panel.PanelItems(imagetpl.NewImageTemplatePage("Promo", "promo").
//	    Background("assets/promo.jpg").
//	    Text("headline", "Headline").Position(600, 420).FontSize(64).
//	    Text("subtitle", "Subtitle").Position(600, 510).FontSize(32).
//	    End())
type ImageTemplatePage struct {
	*panel.CustomPanelPage

	mu              sync.RWMutex
	background      []byte
	backgroundStore BackgroundStore
	bgWidth         int
	bgHeight        int
	bgVersion       int64

	elements []TemplateElement
	fonts    []FontOption
	remember bool
	valuesFn func(*http.Request) map[string]string

	exportHook   ExportHook
	filename     string
	publishLabel string
}

// NewImageTemplatePage constructs an image-template page rooted at the given
// slug. Until you add at least one Text(...) element (or upload a background)
// the page shows an empty state.
func NewImageTemplatePage(name, slug string) *ImageTemplatePage {
	cpp := panel.NewCustomPanelPage(name, slug)
	cpp.Title(name)
	return &ImageTemplatePage{
		CustomPanelPage: cpp,
		filename:        "template.jpg",
		publishLabel:    "Publish",
		fonts:           DefaultFontOptions(),
		remember:        true,
	}
}

// --- builders ----------------------------------------------------------------

// Background loads a default background JPEG from a filesystem path at
// construction time. Errors are logged and the page starts with no background,
// leaving it to the runtime upload control. Use BackgroundBytes for embedded
// images.
func (p *ImageTemplatePage) Background(path string) *ImageTemplatePage {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("[imagetpl] failed to load background %q: %v", path, err)
		return p
	}
	return p.BackgroundBytes(data)
}

// BackgroundBytes sets the default background image from raw bytes. The bytes
// must decode as an image within the configured dimension limit; otherwise they
// are ignored with a logged error.
func (p *ImageTemplatePage) BackgroundBytes(data []byte) *ImageTemplatePage {
	width, height, err := validateBackground(data)
	if err != nil {
		logger.Error("[imagetpl] background bytes are not a decodable image: %v", err)
		return p
	}
	p.setBackground(data, width, height, false)
	return p
}

// ValuesFn supplies initial text for each request. Only configured text fields
// are used, so callers can safely return a shared map with other values.
func (p *ImageTemplatePage) ValuesFn(fn func(*http.Request) map[string]string) *ImageTemplatePage {
	p.mu.Lock()
	p.valuesFn = fn
	p.mu.Unlock()
	return p
}

// Text begins defining a new text element (and commits any previously started
// element). It returns a *TextElementBuilder for fluent configuration; chain
// Position / FontSize / Color / Align / MaxWidth, then start another element
// with Text(...) or finish with End() to get the page back.
func (p *ImageTemplatePage) Text(field, label string) *TextElementBuilder {
	return &TextElementBuilder{
		page: p,
		elem: TemplateElement{
			Field:    field,
			Label:    label,
			FontSize: defaultFontSize,
			Color:    defaultTextColor,
			Align:    AlignLeft,
			Font:     FontRegular,
		},
	}
}

func (p *ImageTemplatePage) addElement(e TemplateElement) {
	e.Field = strings.TrimSpace(e.Field)
	if e.Field == "" {
		return
	}
	if e.Label == "" {
		e.Label = e.Field
	}
	if e.Font == "" || !isBuiltInFont(e.Font) {
		e.Font = FontRegular
	}
	if !isHexColor(e.Color) {
		e.Color = defaultTextColor
	}
	if e.Align != AlignCenter && e.Align != AlignRight {
		e.Align = AlignLeft
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, existing := range p.elements {
		if existing.Field == e.Field {
			logger.Error("[imagetpl] duplicate text field %q ignored", e.Field)
			return
		}
	}
	p.elements = append(p.elements, e)
}

// TextElementBuilder is a fluent sub-builder returned by
// ImageTemplatePage.Text. Config methods return the builder so several can be
// chained; Text starts the next element and End returns to the page.
type TextElementBuilder struct {
	page *ImageTemplatePage
	elem TemplateElement
}

// Position sets the top-left origin of the text box, in background pixels.
func (b *TextElementBuilder) Position(x, y int) *TextElementBuilder {
	b.elem.X = x
	b.elem.Y = y
	b.elem.RelativePosition = false
	return b
}

// PositionRelative places the text anchor at fractions of the background size.
func (b *TextElementBuilder) PositionRelative(x, y float64) *TextElementBuilder {
	if x >= 0 && x <= 1 && y >= 0 && y <= 1 {
		b.elem.XRatio = x
		b.elem.YRatio = y
		b.elem.RelativePosition = true
	}
	return b
}

// FontSize sets the point size (== px at the renderer's 72 DPI).
func (b *TextElementBuilder) FontSize(pt float64) *TextElementBuilder {
	b.elem.FontSize = pt
	b.elem.RelativeFontSize = false
	return b
}

// FontSizeRelative sizes text as a fraction of the background height.
func (b *TextElementBuilder) FontSizeRelative(fraction float64) *TextElementBuilder {
	if fraction > 0 && fraction <= 1 {
		b.elem.FontSizeRatio = fraction
		b.elem.RelativeFontSize = true
	}
	return b
}

// Color sets the text colour as "#RRGGBB".
func (b *TextElementBuilder) Color(hex string) *TextElementBuilder {
	b.elem.Color = hex
	return b
}

// Font sets the default font for this text layer. End users can still choose
// another configured font from the editor.
func (b *TextElementBuilder) Font(font string) *TextElementBuilder {
	b.elem.Font = font
	return b
}

// Align sets the horizontal alignment: AlignLeft / AlignCenter / AlignRight.
func (b *TextElementBuilder) Align(a string) *TextElementBuilder {
	b.elem.Align = a
	return b
}

// MaxWidth enables word wrapping to the given pixel width.
func (b *TextElementBuilder) MaxWidth(px int) *TextElementBuilder {
	b.elem.MaxWidth = px
	b.elem.RelativeMaxWidth = false
	return b
}

// MaxWidthRelative wraps text within a fraction of the background width.
func (b *TextElementBuilder) MaxWidthRelative(fraction float64) *TextElementBuilder {
	if fraction > 0 && fraction <= 1 {
		b.elem.MaxWidthRatio = fraction
		b.elem.RelativeMaxWidth = true
	}
	return b
}

// Text commits the current element and begins defining a new text element.
func (b *TextElementBuilder) Text(field, label string) *TextElementBuilder {
	b.page.addElement(b.elem)
	return b.page.Text(field, label)
}

// End commits the current element and returns the configured page.
func (b *TextElementBuilder) End() *ImageTemplatePage {
	b.page.addElement(b.elem)
	return b.page
}

// --- Panel-page builder wrappers --------------------------------------------
//
// The embedded *CustomPanelPage exposes these, but they return
// *CustomPanelPage which breaks fluent chaining from an *ImageTemplatePage.
// Each wrapper delegates to the embedded method and returns *ImageTemplatePage
// so the whole configuration can be written as one chain.

// Title sets the page title shown at the top of the page (defaults to name).
func (p *ImageTemplatePage) Title(value string) *ImageTemplatePage {
	p.CustomPanelPage.Title(value)
	return p
}

// TitleFn is a callback alias for Title.
func (p *ImageTemplatePage) TitleFn(fn func(ctx *ctxPanel.PanelPage) string) *ImageTemplatePage {
	p.CustomPanelPage.TitleFn(fn)
	return p
}

// Description sets the descriptive subtitle shown beneath the title.
func (p *ImageTemplatePage) Description(value string) *ImageTemplatePage {
	p.CustomPanelPage.Description(value)
	return p
}

// DescriptionFn is a callback alias for Description.
func (p *ImageTemplatePage) DescriptionFn(fn func(ctx *ctxPanel.PanelPage) string) *ImageTemplatePage {
	p.CustomPanelPage.DescriptionFn(fn)
	return p
}

// NavigationLabel overrides the label shown in the sidebar (defaults to name).
func (p *ImageTemplatePage) NavigationLabel(label string) *ImageTemplatePage {
	p.CustomPanelPage.NavigationLabel(label)
	return p
}

// NavigationGroup places this page under a named group in the sidebar.
func (p *ImageTemplatePage) NavigationGroup(group string) *ImageTemplatePage {
	p.CustomPanelPage.NavigationGroup(group)
	return p
}

// NavigationOrder pins the page's sidebar position (lower = earlier).
func (p *ImageTemplatePage) NavigationOrder(order int) *ImageTemplatePage {
	p.CustomPanelPage.NavigationOrder(order)
	return p
}

// NavigationHidden hides this page from the sidebar (still reachable + shown in
// the sub-page tab strip).
func (p *ImageTemplatePage) NavigationHidden() *ImageTemplatePage {
	p.CustomPanelPage.NavigationHidden()
	return p
}

// PageTabHidden hides this page from the page-tab strip while leaving it in the
// sidebar.
func (p *ImageTemplatePage) PageTabHidden() *ImageTemplatePage {
	p.CustomPanelPage.PageTabHidden()
	return p
}

// CanAccess installs an auth gate that runs before the page renders.
func (p *ImageTemplatePage) CanAccess(fn func(ctx *ctxPanel.PanelPage) bool) *ImageTemplatePage {
	p.CustomPanelPage.CanAccess(fn)
	return p
}

// SkipPanel skips the panel middleware defined on your panel.
func (p *ImageTemplatePage) SkipPanel() *ImageTemplatePage {
	p.CustomPanelPage.SkipPanel()
	return p
}

// SkipAuth skips the auth middleware defined on your panel.
func (p *ImageTemplatePage) SkipAuth() *ImageTemplatePage {
	p.CustomPanelPage.SkipAuth()
	return p
}

// --- IPage ------------------------------------------------------------------

// GetComponent renders the editor screen. It builds a fresh per-request page
// snapshot (the layout manager pattern) so the live background state — whether
// one has been uploaded, its dimensions and cache-busting version — is read at
// request time rather than baked in at construction.
func (p *ImageTemplatePage) GetComponent(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
	d := p.buildPageData(r)
	cppCopy := p.cloneCustomPanelPage()
	cppCopy.ContentObj = views.Editor(d)
	return cppCopy.GetComponent(w, r)
}

// RegisterRoutes registers the main page route plus the plugin's own endpoints
// (upload / background / export) under the page's scoped mux. Overriding the
// embedded CustomPanelPage.RegisterRoutes ensures the page handler uses *our*
// GetComponent (the embedded type would otherwise capture its own method and
// miss the dynamic content override).
func (p *ImageTemplatePage) RegisterRoutes(mux wrapper.IMux) {
	scoped := p.GetPageMux(mux)
	pageScoped := scoped
	if p.SlugStr != "" {
		pageScoped = wrapper.NewPrefixMux(scoped, p.SlugStr)
	}

	p.BasePage.RegisterRoutes(scoped, p.GetComponent)

	for _, sub := range p.GetSubPages() {
		sub.RegisterRoutes(scoped)
	}

	p.registerPluginRoutes(pageScoped)
	p.RegisterCarrier(scoped)
}

func (p *ImageTemplatePage) cloneCustomPanelPage() panel.CustomPanelPage {
	page := *p.CustomPanelPage
	resolvable := *p.CustomPanelPage.PanelPageResolvable
	page.PanelPageResolvable = &resolvable
	return page
}

// --- view data --------------------------------------------------------------

func (p *ImageTemplatePage) buildPageData(r *http.Request) *views.PageData {
	prefix := panelPathFromRequest(r) + p.SlugStr

	p.mu.RLock()
	hasBg := len(p.background) > 0
	bgW := p.bgWidth
	bgH := p.bgHeight
	version := p.bgVersion
	hasPublish := p.exportHook != nil
	publishLabel := p.publishLabel
	remember := p.remember
	valuesFn := p.valuesFn
	elements := append([]TemplateElement(nil), p.elements...)
	fonts := cloneFontOptions(p.fonts)
	p.mu.RUnlock()

	d := &views.PageData{
		HasBackground:    hasBg,
		BackgroundWidth:  bgW,
		BackgroundHeight: bgH,
		HasPublish:       hasPublish,
		PublishLabel:     publishLabel,
		RememberValues:   remember,
		FieldDefaults:    make(map[string]string, len(elements)),
		PersistenceKey:   persistenceKey(prefix),
		URLs: views.PageURLs{
			Upload:  prefix + routeUpload,
			Export:  prefix + routeExport,
			Publish: prefix + routePublish,
		},
	}
	if hasBg {
		d.BackgroundURL = prefix + routeBackground + "?v=" + strconv.FormatInt(version, 10)
	}

	for _, el := range elements {
		el = resolveGeometry(el, bgW, bgH)
		font := submittedFont("", el.Font, fonts)
		d.Elements = append(d.Elements, views.ElementData{
			Field:    el.Field,
			Label:    el.Label,
			X:        el.X,
			Y:        el.Y,
			FontSize: el.FontSize,
			Color:    el.Color,
			Align:    el.Align,
			Font:     font,
			MaxWidth: el.MaxWidth,
		})
	}
	if valuesFn != nil {
		defaults := valuesFn(r)
		for _, el := range elements {
			d.FieldDefaults[el.Field] = limitText(defaults[el.Field])
		}
	}
	for _, font := range fonts {
		d.Fonts = append(d.Fonts, views.FontData{ID: font.ID, Label: font.Label})
	}
	return d
}

type pageSnapshot struct {
	background   []byte
	elements     []TemplateElement
	fonts        []FontOption
	filename     string
	publishLabel string
	exportHook   ExportHook
	bgWidth      int
	bgHeight     int
}

func (p *ImageTemplatePage) snapshot() pageSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return pageSnapshot{
		background:   append([]byte(nil), p.background...),
		elements:     append([]TemplateElement(nil), p.elements...),
		fonts:        cloneFontOptions(p.fonts),
		filename:     p.filename,
		publishLabel: p.publishLabel,
		exportHook:   p.exportHook,
		bgWidth:      p.bgWidth,
		bgHeight:     p.bgHeight,
	}
}

func (p *ImageTemplatePage) setBackground(data []byte, width, height int, incrementVersion bool) {
	copyData := append([]byte(nil), data...)
	p.mu.Lock()
	p.background = copyData
	p.bgWidth = width
	p.bgHeight = height
	if incrementVersion {
		p.bgVersion++
	}
	p.mu.Unlock()
}

func (p *ImageTemplatePage) saveBackground(ctx context.Context, data []byte, width, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.backgroundStore != nil {
		if err := p.backgroundStore.Save(ctx, data); err != nil {
			return err
		}
	}
	p.background = append([]byte(nil), data...)
	p.bgWidth = width
	p.bgHeight = height
	p.bgVersion++
	return nil
}

// panelPathFromRequest pulls the panel prefix from the current request's
// context. Iridium sets ctxkeys.PanelPath on every request that goes through a
// panel mux; if missing we fall back to "" (root-mounted page).
func panelPathFromRequest(r *http.Request) string {
	v := r.Context().Value(ctxkeys.PanelPath)
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimRight(s, "/")
}

var _ templ.Component = templ.NopComponent
