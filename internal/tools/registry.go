package tools

import (
	"log/slog"

	"github.com/LanthornHQ/iris/internal/browser"
	"github.com/LanthornHQ/iris/internal/grounding"
	"github.com/LanthornHQ/iris/internal/mcp"
)

type ToolRegistry struct {
	Logger    *slog.Logger
	Driver    browser.Driver
	Grounding *grounding.Client
}

func NewToolRegistry(logger *slog.Logger, driver browser.Driver, groundingClient *grounding.Client) *ToolRegistry {
	return &ToolRegistry{
		Logger:    logger,
		Driver:    driver,
		Grounding: groundingClient,
	}
}

// RegisterAll registers all 9 browser-control tools on the MCP server.
func (r *ToolRegistry) RegisterAll(s *mcp.Server) {
	s.RegisterTool(&Navigate{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Screenshot{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&Click{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&TypeText{Logger: r.Logger, Driver: r.Driver, Grounding: r.Grounding})
	s.RegisterTool(&Scroll{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&WaitForStable{Logger: r.Logger, Driver: r.Driver})
	s.RegisterTool(&VerifyScreen{Logger: r.Logger, Driver: r.Driver, Verifier: r.Grounding})
	s.RegisterTool(&Sleep{Logger: r.Logger})
	s.RegisterTool(&GetDatetime{Logger: r.Logger})
}
