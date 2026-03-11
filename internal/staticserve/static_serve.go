package staticserve

import (
	"embed"
	"net/http"

	"github.com/HexmosTech/git-lrc/result"
)

//go:embed static/*
var staticFiles embed.FS

type JSONTemplateData = result.JSONTemplateData
type JSONFileData = result.JSONFileData
type JSONHunkData = result.JSONHunkData
type JSONLineData = result.JSONLineData
type JSONCommentData = result.JSONCommentData

// RenderPreactHTML renders the Preact-based HTML with embedded JSON data.
func RenderPreactHTML(data *result.HTMLTemplateData) (string, error) {
	return result.RenderPreactHTML(data, staticFiles)
}

// GetStaticHandler returns an HTTP handler for serving static files.
func GetStaticHandler() http.Handler {
	return result.GetStaticHandler(staticFiles)
}

// ServeStaticFile serves a specific static file.
func ServeStaticFile(w http.ResponseWriter, r *http.Request, filename string) error {
	return result.ServeStaticFile(w, filename, staticFiles)
}

// ReadFile reads a file from the embedded static directory.
func ReadFile(name string) ([]byte, error) {
	return staticFiles.ReadFile("static/" + name)
}
