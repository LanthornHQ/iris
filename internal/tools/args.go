package tools

import (
	"fmt"
	"math"
)

func intArg(args map[string]any, name string) (int, error) {
	v, ok := args[name]
	if !ok {
		return 0, fmt.Errorf("%s is required and must be an integer", name)
	}
	var floatVal float64
	switch typedVal := v.(type) {
	case float64:
		floatVal = typedVal
	case int:
		floatVal = float64(typedVal)
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", name, v)
	}

	if floatVal < 0 || floatVal > float64(math.MaxInt32) {
		return 0, fmt.Errorf("%s is out of range: %v", name, floatVal)
	}
	return int(floatVal), nil
}

func optIntArg(args map[string]any, name string, defaultVal int) int {
	v, ok := args[name]
	if !ok {
		return defaultVal
	}
	var floatVal float64
	switch typedVal := v.(type) {
	case float64:
		floatVal = typedVal
	case int:
		floatVal = float64(typedVal)
	default:
		return defaultVal
	}

	if floatVal < 0 || floatVal > float64(math.MaxInt32) {
		return defaultVal
	}
	return int(floatVal)
}
