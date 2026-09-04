package web

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/web/static"
)

// AssetCacheControl marks both embedded assets immutable: they're compiled
// into the binary, so a running instance can never serve two different
// bytes under the same URL.
const AssetCacheControl = "public, max-age=31536000, immutable"

func ServeAppCSS(e *core.RequestEvent) error {
	return serveAsset(e, "text/css; charset=utf-8", static.AppCSS)
}

func ServeDatastarJS(e *core.RequestEvent) error {
	return serveAsset(e, "text/javascript; charset=utf-8", static.Datastar)
}

func serveAsset(e *core.RequestEvent, contentType string, body []byte) error {
	e.Response.Header().Set("Cache-Control", AssetCacheControl)

	return e.Blob(http.StatusOK, contentType, body)
}
