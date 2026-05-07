package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "iris_tool_calls_total",
		Help: "Total tool calls handled by this node by tool and status.",
	}, []string{"tool", "status"})

	ToolCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "iris_tool_call_duration_seconds",
		Help:    "Duration of tool execution.",
		Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
	}, []string{"tool"})

	GroundingRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "iris_grounding_request_duration_seconds",
		Help:    "Duration of vision grounding model requests.",
		Buckets: []float64{.1, .25, .5, 1, 2, 5, 10, 30, 60},
	})
)
