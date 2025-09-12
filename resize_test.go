package main

import (
	"testing"
	
	"github.com/h2non/bimg"
)

// TestCorrectResizeLogic tests the fixed resize logic with PNG to JPEG conversion
func TestCorrectResizeLogic(t *testing.T) {
	// Load PNG test image
	originalImage, err := bimg.Read("HappyNotes.png")
	if err != nil {
		t.Fatalf("Failed to load HappyNotes.png: %v", err)
	}
	
	oldImage := bimg.NewImage(originalImage)
	oldImageSize, err := oldImage.Size()
	if err != nil {
		t.Fatalf("Failed to get image size: %v", err)
	}
	
	t.Logf("Original PNG: %dx%d, %d bytes", oldImageSize.Width, oldImageSize.Height, len(originalImage))
	
	// Test with corrected resize logic
	maxWidth, maxHeight := 800, 600
	needsResize := oldImageSize.Width > maxWidth || oldImageSize.Height > maxHeight
	
	var newWidth, newHeight int
	if needsResize {
		t.Logf("Resize needed: %dx%d exceeds %dx%d", oldImageSize.Width, oldImageSize.Height, maxWidth, maxHeight)
		
		scaleWidth := float64(maxWidth) / float64(oldImageSize.Width)
		scaleHeight := float64(maxHeight) / float64(oldImageSize.Height)
		scale := scaleWidth
		if scaleHeight < scaleWidth {
			scale = scaleHeight
		}
		
		newWidth = int(float64(oldImageSize.Width) * scale)
		newHeight = int(float64(oldImageSize.Height) * scale)
		t.Logf("Scale: %.3f, New size: %dx%d", scale, newWidth, newHeight)
	} else {
		newWidth = oldImageSize.Width
		newHeight = oldImageSize.Height
		t.Log("No resize needed")
	}
	
	// Test different quality levels
	qualities := []int{30, 50, 75, 95}
	
	for _, quality := range qualities {
		options := bimg.Options{
			Width:   newWidth,
			Height:  newHeight,
			Quality: quality,
			Type:    bimg.JPEG,
		}
		
		result, err := oldImage.Process(options)
		if err != nil {
			t.Fatalf("Processing failed for quality %d: %v", quality, err)
		}
		
		// Verify result
		processedImage := bimg.NewImage(result)
		processedSize, err := processedImage.Size()
		if err != nil {
			t.Fatalf("Failed to get processed size: %v", err)
		}
		
		t.Logf("Quality %d: %dx%d, %d bytes", quality, processedSize.Width, processedSize.Height, len(result))
		
		// Verify dimensions are within limits
		if processedSize.Width > maxWidth || processedSize.Height > maxHeight {
			t.Errorf("Quality %d: dimensions %dx%d exceed limits %dx%d", 
				quality, processedSize.Width, processedSize.Height, maxWidth, maxHeight)
		}
	}
}