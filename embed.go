package imagetpl

import (
	"embed"

	"github.com/iridiumgo/iridium/core/asset"
)

//go:embed static
var staticFS embed.FS

func init() {
	asset.Register(
		asset.CSS("imagetpl", staticFS, "static/imagetpl.css").Package("imagetpl"),
		asset.JS("imagetpl", staticFS, "static/imagetpl.js").Package("imagetpl"),
	)
}
