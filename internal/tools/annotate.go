package tools

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
)

const (
	clickDotRadius     = 12
	ringBorderWidth    = 2
	crossLengthPadding = 4
	defaultJPEGQuality = 85
)

type dotInfo struct {
	x int
	y int
	c color.Color
}

func drawRing(rgba *image.RGBA, px, py, radius int, c color.Color) {
	for dy := -(radius + ringBorderWidth); dy <= radius+ringBorderWidth; dy++ {
		for dx := -(radius + ringBorderWidth); dx <= radius+ringBorderWidth; dx++ {
			cx, cy := px+dx, py+dy
			if cx < rgba.Bounds().Min.X || cx >= rgba.Bounds().Max.X || cy < rgba.Bounds().Min.Y || cy >= rgba.Bounds().Max.Y {
				continue
			}
			dist2 := dx*dx + dy*dy
			outerR2 := (radius + ringBorderWidth) * (radius + ringBorderWidth)
			innerR2 := (radius - 1) * (radius - 1)
			if dist2 <= outerR2 && dist2 >= innerR2 {
				rgba.Set(cx, cy, c)
			}
		}
	}
}

func drawInnerDot(rgba *image.RGBA, px, py, radius int, c color.Color) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				cx, cy := px+dx, py+dy
				if cx >= rgba.Bounds().Min.X && cx < rgba.Bounds().Max.X && cy >= rgba.Bounds().Min.Y && cy < rgba.Bounds().Max.Y {
					rgba.Set(cx, cy, c)
				}
			}
		}
	}
}

func drawCross(rgba *image.RGBA, px, py, radius int, c color.Color) {
	crossLen := radius + crossLengthPadding
	for dx := -crossLen; dx <= crossLen; dx++ {
		cx := px + dx
		if cx >= rgba.Bounds().Min.X && cx < rgba.Bounds().Max.X && py >= rgba.Bounds().Min.Y && py < rgba.Bounds().Max.Y {
			rgba.Set(cx, py, c)
		}
	}
	for dy := -crossLen; dy <= crossLen; dy++ {
		cy := py + dy
		if px >= rgba.Bounds().Min.X && px < rgba.Bounds().Max.X && cy >= rgba.Bounds().Min.Y && cy < rgba.Bounds().Max.Y {
			rgba.Set(px, cy, c)
		}
	}
}

func drawSingleDot(rgba *image.RGBA, d dotInfo, radius int, scaleX, scaleY float64) {
	px := int(float64(d.x) * scaleX)
	py := int(float64(d.y) * scaleY)
	white := color.RGBA{255, 255, 255, 255}

	drawRing(rgba, px, py, radius, white)
	drawInnerDot(rgba, px, py, radius, d.c)
	drawCross(rgba, px, py, radius, d.c)
}

func drawDotsOnScreenshot(screenshotB64 string, imgW, imgH int, dots []dotInfo, radius int) (string, error) {
	imgData, err := base64.StdEncoding.DecodeString(screenshotB64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	srcImg, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return "", fmt.Errorf("image decode: %w", err)
	}

	rgba, ok := srcImg.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(srcImg.Bounds())
		draw.Draw(rgba, rgba.Bounds(), srcImg, srcImg.Bounds().Min, draw.Src)
	}

	scaleX := float64(rgba.Bounds().Dx()) / float64(imgW)
	scaleY := float64(rgba.Bounds().Dy()) / float64(imgH)

	for _, d := range dots {
		drawSingleDot(rgba, d, radius, scaleX, scaleY)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: defaultJPEGQuality}); err != nil {
		return "", fmt.Errorf("jpeg encode: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func drawClickDot(screenshotB64 string, clickX, clickY, imgW, imgH int) (string, error) {
	red := color.RGBA{255, 0, 0, 255}
	dots := []dotInfo{{clickX, clickY, red}}
	return drawDotsOnScreenshot(screenshotB64, imgW, imgH, dots, clickDotRadius)
}
