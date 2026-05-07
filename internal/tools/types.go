package tools

import (
	"context"

	"github.com/LanthornHQ/iris/internal/grounding"
)

const ConfidenceVision = 0.5

type Image = grounding.Image

type BoundingBox = grounding.BoundingBox

// GroundingClient resolves a natural-language description to screen coordinates.
type GroundingClient interface {
	Ground(ctx context.Context, target string, intent string, img Image) (BoundingBox, error)
}

// VerifyClient answers yes/no questions about a screenshot.
type VerifyClient interface {
	Verify(ctx context.Context, question string, img Image) (answer, evidence string, bbox BoundingBox, err error)
}
