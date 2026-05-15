package tools

import (
	"log/slog"
	"os"

	"github.com/LanthornHQ/iris/internal/browser"
	"github.com/LanthornHQ/iris/internal/mcp"
)

type ToolRegistry struct {
	Logger *slog.Logger
	Driver browser.Driver
}

func NewToolRegistry(logger *slog.Logger, driver browser.Driver) *ToolRegistry {
	return &ToolRegistry{
		Logger: logger,
		Driver: driver,
	}
}

// RegisterAll registers all browser-control tools on the MCP server.
func (r *ToolRegistry) RegisterAll(s *mcp.Server) {
	annotateDefault := os.Getenv("IRIS_ANNOTATE_CLICKS") == "1"

	s.RegisterTool(&Navigate{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Screenshot{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Click{Logger: r.Logger, Driver: r.Driver, AnnotateDefault: annotateDefault})
	s.RegisterTool(&TypeText{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Scroll{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&WaitForStable{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Sleep{Logger: r.Logger})
	s.RegisterTool(&GetDatetime{Logger: r.Logger})
	s.RegisterTool(&DesktopScreenshot{Logger: r.Logger})
}
