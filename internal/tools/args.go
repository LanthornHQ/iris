package tools

import (
	"fmt"
	"math"
)

func intArg(args map[string]any, name string) (int, error) {
	v, ok := args[name].(float64)
	if !ok {
		return 0, fmt.Errorf("%s is required and must be an integer", name)
	}
	if v < 0 || v > float64(math.MaxInt32) {
		return 0, fmt.Errorf("%s is out of range: %v", name, v)
	}
	return int(v), nil
}

func optIntArg(args map[string]any, name string, defaultVal int) int {
	v, ok := args[name].(float64)
	if !ok {
		return defaultVal
	}
	if v < 0 || v > float64(math.MaxInt32) {
		return defaultVal
	}
	return int(v)
}

func optBoolArg(args map[string]any, name string, defaultVal bool) bool {
	v, ok := args[name].(bool)
	if !ok {
		return defaultVal
	}
	return v
}
