package ui

import (
	"net/http"

	"github.com/upspeak/upspeak/app"
)

// Implements app.Module interface
type ModuleUI struct{}

func (m ModuleUI) Name() string {
	return "ui"
}

func (m ModuleUI) Init(config map[string]any) error {
	// Initialization logic for the writer module
	return nil
}

func (m ModuleUI) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
	// Return HTTP handlers for the writer module
	return []app.HTTPHandler{
		{
			Method: "GET",
			Path:   "/",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("UI Module"))
			},
		},
	}
}

func (m ModuleUI) MsgHandlers(pub app.Publisher) []app.MsgHandler {
	// Return message handlers for the writer module
	return []app.MsgHandler{}
}
