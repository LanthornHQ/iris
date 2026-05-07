package tools

import (
	"context"
	"log/slog"
	"time"
)

type GetDatetime struct {
	Logger *slog.Logger
}

func (t *GetDatetime) Name() string { return "get_datetime" }

func (t *GetDatetime) Description() string {
	return `Get the current date and time of the execution node. Use this when you need to know today's date for selecting dates in the UI (e.g., booking flights or hotels).`
}

func (t *GetDatetime) ParametersSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *GetDatetime) Execute(ctx context.Context, _ map[string]any) (any, error) {
	now := time.Now()

	if t.Logger != nil {
		t.Logger.InfoContext(ctx, "get_datetime called", "time", now.Format(time.RFC3339))
	}

	return GetDatetimeResponse{
		Datetime: now.Format(time.RFC3339),
		Date:     now.Format("2006-01-02"),
		Time:     now.Format("15:04:05"),
		Weekday:  now.Weekday().String(),
	}, nil
}
