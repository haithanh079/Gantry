// Package mcp exposes Gantry's catalog over the Model Context Protocol so local
// AI agents (Cursor, Claude Code, etc.) can read and update services.
//
// The transport is Streamable HTTP. Clients authenticate with an API key, a
// browser session, or an MCP OAuth access token obtained by granting access in
// the Gantry UI.
package mcp

import (
	"net/http"

	"github.com/go2engle/gantry/internal/api/handlers"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHandler builds an http.Handler that serves MCP over Streamable HTTP,
// exposing tools backed by the given Handlers' services.
func NewHandler(h *handlers.Handlers) http.Handler {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "gantry",
		Version: h.Version,
	}, nil)

	registerTools(srv, h)

	return mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return srv
	}, nil)
}
