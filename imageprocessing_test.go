package main

import (
	"testing"
)

// Helper function to create ImageProcessingSettings from individual values for testing
func createImageProcessingSettings(maxWidth, maxHeight, maxNarrowSide, jpegQuality, webpQuality int, convertToFormat string) ImageProcessingSettings {
	return ImageProcessingSettings{
		MaxWidth:        maxWidth,
		MaxHeight:       maxHeight,
		MaxNarrowSide:   maxNarrowSide,
		JpegQuality:     jpegQuality,
		WebpQuality:     webpQuality,
		ConvertToFormat: convertToFormat,
	}
}

func TestCalculateNarrowSideResize(t *testing.T) {
	tests := []struct {
		name           string
		original       ImageSize
		maxNarrowSide  int
		expected       ImageSize
	}{
		{
			name:          "No resize needed - within limit",
			original:      ImageSize{Width: 800, Height: 600},
			maxNarrowSide: 600,
			expected:      ImageSize{Width: 800, Height: 600},
		},
		{
			name:          "Resize landscape image - height is narrow side",
			original:      ImageSize{Width: 1600, Height: 800},
			maxNarrowSide: 400,
			expected:      ImageSize{Width: 800, Height: 400},
		},
		{
			name:          "Resize portrait image - width is narrow side",
			original:      ImageSize{Width: 600, Height: 1200},
			maxNarrowSide: 300,
			expected:      ImageSize{Width: 300, Height: 600},
		},
		{
			name:          "Square image resize",
			original:      ImageSize{Width: 1000, Height: 1000},
			maxNarrowSide: 500,
			expected:      ImageSize{Width: 500, Height: 500},
		},
		{
			name:          "Very narrow image",
			original:      ImageSize{Width: 2000, Height: 100},
			maxNarrowSide: 50,
			expected:      ImageSize{Width: 1000, Height: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNarrowSideResize(tt.original, tt.maxNarrowSide)
			if result.Width != tt.expected.Width || result.Height != tt.expected.Height {
				t.Errorf("calculateNarrowSideResize() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateBoundingBoxResize(t *testing.T) {
	tests := []struct {
		name      string
		original  ImageSize
		maxWidth  int
		maxHeight int
		expected  ImageSize
	}{
		{
			name:      "No resize needed - within bounds",
			original:  ImageSize{Width: 500, Height: 300},
			maxWidth:  800,
			maxHeight: 600,
			expected:  ImageSize{Width: 500, Height: 300},
		},
		{
			name:      "Resize by width constraint",
			original:  ImageSize{Width: 1600, Height: 800},
			maxWidth:  800,
			maxHeight: 1000,
			expected:  ImageSize{Width: 800, Height: 400},
		},
		{
			name:      "Resize by height constraint",
			original:  ImageSize{Width: 800, Height: 1200},
			maxWidth:  1000,
			maxHeight: 600,
			expected:  ImageSize{Width: 400, Height: 600},
		},
		{
			name:      "Resize by both constraints - width tighter",
			original:  ImageSize{Width: 2000, Height: 1000},
			maxWidth:  400,
			maxHeight: 800,
			expected:  ImageSize{Width: 400, Height: 200},
		},
		{
			name:      "Resize by both constraints - height tighter",
			original:  ImageSize{Width: 1000, Height: 2000},
			maxWidth:  800,
			maxHeight: 400,
			expected:  ImageSize{Width: 200, Height: 400},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateBoundingBoxResize(tt.original, tt.maxWidth, tt.maxHeight)
			if result.Width != tt.expected.Width || result.Height != tt.expected.Height {
				t.Errorf("calculateBoundingBoxResize() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateResizeDimensions(t *testing.T) {
	tests := []struct {
		name     string
		original ImageSize
		settings ImageProcessingSettings
		expected ImageSize
	}{
		{
			name:     "Use narrow side strategy",
			original: ImageSize{Width: 1200, Height: 800},
			settings: createImageProcessingSettings(1000, 1000, 400, 80, 85, ""),
			expected: ImageSize{Width: 600, Height: 400},
		},
		{
			name:     "Use bounding box strategy - no narrow side set",
			original: ImageSize{Width: 1200, Height: 800},
			settings: createImageProcessingSettings(600, 1000, 0, 80, 85, ""), // MaxNarrowSide=0 means not set
			expected: ImageSize{Width: 600, Height: 400},
		},
		{
			name:     "Narrow side strategy takes precedence",
			original: ImageSize{Width: 1000, Height: 2000},
			settings: createImageProcessingSettings(500, 500, 800, 80, 85, ""), // Narrow side looser but takes precedence
			expected: ImageSize{Width: 800, Height: 1600},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateResizeDimensions(tt.original, tt.settings)
			if result.Width != tt.expected.Width || result.Height != tt.expected.Height {
				t.Errorf("calculateResizeDimensions() = %v, want %v", result, tt.expected)
			}
		})
	}
}