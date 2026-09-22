// Package imagetpl is an Iridium plugin that turns a template image into a
// fill-in-the-blanks image generator. An admin uploads (or configures) a
// background image — say a promotional banner with the company logo already
// baked into the corner — and defines a set of dynamic text elements on top of
// it. End users then type the text and export a flattened JPEG, server-rendered
// with Go's image stack.
//
// The plugin keeps the architecture deliberately simple: the background is treated as
// a static background and every dynamic piece of content is a text layer drawn
// over it at export time. The uploaded background is never modified; a new JPEG
// is composited on demand and streamed back to the browser. Text, font, and
// colour choices are remembered in the user's browser by default and can be
// disabled with RememberValues(false).
//
// Basic usage:
//
//	promo := imagetpl.NewImageTemplatePage("Promo Banner", "promo").
//	    Background("assets/promo.jpg").
//	    Text("company_name", "Company Name").
//	        Position(600, 420).
//	        FontSize(64).
//	        Align(imagetpl.AlignCenter).
//	    Text("promo_text", "Promotional Message").
//	        Position(600, 510).
//	        FontSize(32).
//	        Align(imagetpl.AlignCenter).
//	    End()
//
//	panel.PanelItems(promo)
//
// Note: your panel must apply the standard panel middleware for the page to
// render inside the Iridium app shell (sidebar, <head>, and therefore this
// plugin's CSS). At minimum add middleware.Spa to your PanelMiddleware:
//
//	panel.NewPanel(mux).
//	    PanelItems(promo).
//	    Middleware(middleware.RecoveryMiddleware, middleware.MaxRequestBodySize).
//	    PanelMiddleware(middleware.Compression, middleware.Spa, middleware.ErrorPages)
//
// The generated screen shows a live HTML/CSS preview (with Alpine for instant
// text and style updates) next to a form of the fields the user is allowed to
// change, an upload control for the background, and an "Export JPEG" button
// that streams the authoritative server-rendered image.
//
// Text elements only for now. Image/logo overlay elements are a natural
// extension of the same TemplateElement model (add a Type field and an image
// draw branch in RenderJPEG); the background is where you'd typically bake a
// company logo today.
package imagetpl
