// Command imagetpl-test is a tiny single-page Iridium app that previews the
// image-template plugin: upload a background, edit text with a live preview,
// download the flattened JPEG, and "publish" it through a server-side hook.
//
// Run it with:
//
//	go run ./test
//
// then open http://127.0.0.1:8099/admin/promo
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/iridiumgo/iridium/config"
	"github.com/iridiumgo/iridium/core/iridium"
	"github.com/iridiumgo/iridium/core/panel"
	"github.com/iridiumgo/iridium/network/middleware"

	imagetpl "github.com/asosick/iridium_image_template"
)

func main() {
	mux := http.NewServeMux()
	app := iridium.NewIridiumApp(appConfig()).RegisterPanels(panelFor(mux)).Initialize(mux)

	log.Println("image-template test app: http://127.0.0.1:8099/admin/promo")
	app.Serve(mux)
}

func appConfig() *config.IridiumConfig {
	cfg := config.NewIridiumConfig()
	cfg.AppName = "Image Template Test App"
	cfg.AppHost = "127.0.0.1"
	cfg.AppPort = "8099"
	// Development-only keys. Generate your own with:
	//   openssl rand -base64 64   (AppKey)
	//   openssl rand -base64 32   (AppEncryptionKey)
	cfg.AppKey = "I0jo08d4+pV81Vi+loqpY8EykKWke//CmX7W01epTYMt6Y8eqCd1RaNmUEqHuR6BJeRzgD45vIxH7gHiu78ijQ=="
	cfg.AppEncryptionKey = "Dojqujyr0RBnfbUT1HFmp/uXzWGN9bRLzeaKTl9ps0A="
	cfg.AppEnv = "development"
	return cfg
}

func panelFor(mux *http.ServeMux) *panel.Panel {
	return panel.NewPanel(mux).
		ID("image-template-test").
		Path("admin").
		NavStyle(panel.NavLeft).
		PanelItems(promoPage()).
		Middleware(
			middleware.RecoveryMiddleware,
			middleware.CSRFMiddleware,
			middleware.MaxRequestBodySize,
		).
		PanelMiddleware(
			middleware.Compression,
			middleware.Spa,
			middleware.ErrorPages,
		)
}

func promoPage() *imagetpl.ImageTemplatePage {
	return imagetpl.NewImageTemplatePage("Promo Banner", "promo").
		Title("Promo Banner").
		Description("Fill in the copy, adjust the styling, then download or publish the banner.").
		BackgroundBytes(placeholderBackground()).
		Filename("promo.jpg").
		PublishLabel("Publish to server").
		ExportHook(publishToDisk).
		Text("company_name", "Company Name").
		Position(600, 250).
		FontSize(58).
		Font(imagetpl.FontBold).
		Align(imagetpl.AlignCenter).
		MaxWidth(980).
		Text("promo_text", "Promotional Message").
		Position(600, 345).
		FontSize(30).
		Font(imagetpl.FontRegular).
		Color("#ffffff").
		Align(imagetpl.AlignCenter).
		MaxWidth(820).
		Text("offer", "Offer Line").
		Position(600, 435).
		FontSize(26).
		Font(imagetpl.FontBold).
		Color("#fde68a").
		Align(imagetpl.AlignCenter).
		MaxWidth(720).
		End().
		SkipAuth()
}

// publishToDisk is an example ExportHook. It receives the rendered JPEG and
// writes it to ./exports/. Swap the body for your real integration (FTP upload,
// S3 PutObject, an HTTP POST to a media service, ...).
func publishToDisk(_ *http.Request, result imagetpl.ExportResult) error {
	dir := "exports"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("promo-%s.jpg", time.Now().Format("20060102-150405"))
	if err := os.WriteFile(filepath.Join(dir, name), result.Data, 0o644); err != nil {
		return err
	}
	log.Printf("published %s (%dx%d) to %s", name, result.Width, result.Height, dir)
	return nil
}

// placeholderBackground generates a simple 1200x628 gradient banner with a
// white "logo" block in the top-left corner, so the app has something to
// composite against without shipping a binary asset.
func placeholderBackground() []byte {
	const w, h = 1200, 628
	top := color.RGBA{R: 0x1e, G: 0x3a, B: 0x8a, A: 0xff}
	bottom := color.RGBA{R: 0x6d, G: 0x28, B: 0xd9, A: 0xff}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		c := lerp(top, bottom, t)
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}

	logo := image.Rect(48, 48, 220, 168)
	draw.Draw(img, logo, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(64, 64, 204, 152),
		&image.Uniform{C: color.RGBA{R: 0x1e, G: 0x3a, B: 0x8a, A: 0xff}},
		image.Point{}, draw.Src)

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92})
	return buf.Bytes()
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t)
	}
	return color.RGBA{
		R: lerp(a.R, b.R),
		G: lerp(a.G, b.G),
		B: lerp(a.B, b.B),
		A: 0xff,
	}
}
