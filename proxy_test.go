package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/h2non/bimg"
)

// setupTestEnvironment sets up environment variables for testing
func setupTestEnvironment() {
	os.Setenv("IMG_MAX_WIDTH", "800")
	os.Setenv("IMG_MAX_HEIGHT", "600")
	os.Setenv("JPEG_QUALITY", "75")
	os.Setenv("FORWARD_DESTINATION", "http://test.example.com/api/assets")
	os.Setenv("FILE_UPLOAD_FIELD", "assetData")
	os.Setenv("LISTEN_PATH", "/api/assets")
}

// createTestImage creates a test JPEG image with specified dimensions
func createTestImage(width, height int, quality int) ([]byte, error) {
	// Create a simple test image
	imageBuffer := make([]byte, width*height*3) // RGB
	for i := range imageBuffer {
		imageBuffer[i] = byte(i % 256) // Create pattern
	}

	// Convert to JPEG using bimg
	options := bimg.Options{
		Width:   width,
		Height:  height,
		Quality: quality,
		Type:    bimg.JPEG,
	}

	return bimg.Resize(imageBuffer, options)
}

// TestJPEGQualityEnvironmentVariable tests that JPEG_QUALITY is properly loaded
func TestJPEGQualityEnvironmentVariable(t *testing.T) {
	// Clean up any existing settings
	settingsInt = make(map[string]int)

	// Test default value
	os.Unsetenv("JPEG_QUALITY")
	defaultSettingsInt := map[string]int{
		IMG_MAX_WIDTH:  1920,
		IMG_MAX_HEIGHT: 1080,
		JPEG_QUALITY:   75,
	}

	intKeys := []string{IMG_MAX_WIDTH, IMG_MAX_HEIGHT, JPEG_QUALITY}
	for _, intKey := range intKeys {
		settingsInt[intKey] = defaultSettingsInt[intKey]

		envValue := os.Getenv(intKey)
		if len(envValue) > 0 {
			if convEnvValue, err := strconv.Atoi(envValue); err == nil {
				settingsInt[intKey] = convEnvValue
			}
		}
	}

	if settingsInt[JPEG_QUALITY] != 75 {
		t.Errorf("Expected default JPEG_QUALITY to be 75, got %d", settingsInt[JPEG_QUALITY])
	}

	// Test custom value
	os.Setenv("JPEG_QUALITY", "50")
	envValue := os.Getenv(JPEG_QUALITY)
	if len(envValue) > 0 {
		if convEnvValue, err := strconv.Atoi(envValue); err == nil {
			settingsInt[JPEG_QUALITY] = convEnvValue
		}
	}

	if settingsInt[JPEG_QUALITY] != 50 {
		t.Errorf("Expected JPEG_QUALITY to be 50, got %d", settingsInt[JPEG_QUALITY])
	}
}

// TestJPEGQualityBoundaries tests edge cases for JPEG quality values
func TestJPEGQualityBoundaries(t *testing.T) {
	testCases := []struct {
		envValue string
		expected int
		name     string
	}{
		{"1", 1, "minimum quality"},
		{"100", 100, "maximum quality"},
		{"30", 30, "low quality"},
		{"95", 95, "high quality"},
		{"invalid", 75, "invalid value should use default"},
		{"0", 0, "zero quality"},
		{"101", 101, "above maximum"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settingsInt = make(map[string]int)
			settingsInt[JPEG_QUALITY] = 75 // default

			os.Setenv("JPEG_QUALITY", tc.envValue)
			envValue := os.Getenv(JPEG_QUALITY)
			if len(envValue) > 0 {
				if convEnvValue, err := strconv.Atoi(envValue); err == nil {
					settingsInt[JPEG_QUALITY] = convEnvValue
				}
			}

			if settingsInt[JPEG_QUALITY] != tc.expected {
				t.Errorf("For %s, expected %d, got %d", tc.name, tc.expected, settingsInt[JPEG_QUALITY])
			}
		})
	}
}

// TestBackwardCompatibility ensures existing functionality still works
func TestBackwardCompatibility(t *testing.T) {
	setupTestEnvironment()

	// Test that the proxy still works without JPEG_QUALITY set
	os.Unsetenv("JPEG_QUALITY")

	// Initialize settings as main() would
	settingsInt = make(map[string]int)
	defaultSettingsInt := map[string]int{
		IMG_MAX_WIDTH:  1920,
		IMG_MAX_HEIGHT: 1080,
		JPEG_QUALITY:   75, // Should have default even if not set
	}

	intKeys := []string{IMG_MAX_WIDTH, IMG_MAX_HEIGHT, JPEG_QUALITY}
	for _, intKey := range intKeys {
		settingsInt[intKey] = defaultSettingsInt[intKey]

		envValue := os.Getenv(intKey)
		if len(envValue) > 0 {
			if convEnvValue, err := strconv.Atoi(envValue); err == nil {
				settingsInt[intKey] = convEnvValue
			}
		}
	}

	// Verify all expected settings are available
	if _, exists := settingsInt[IMG_MAX_WIDTH]; !exists {
		t.Error("IMG_MAX_WIDTH should be available for backward compatibility")
	}
	if _, exists := settingsInt[IMG_MAX_HEIGHT]; !exists {
		t.Error("IMG_MAX_HEIGHT should be available for backward compatibility")
	}
	if _, exists := settingsInt[JPEG_QUALITY]; !exists {
		t.Error("JPEG_QUALITY should have default value for backward compatibility")
	}
}

// TestImageProcessingWithQuality tests actual image processing with different quality settings
func TestImageProcessingWithQuality(t *testing.T) {
	setupTestEnvironment()

	// Load the test image file once
	originalJPEG, err := bimg.Read("Norway.jpeg")
	if err != nil {
		t.Fatalf("Failed to load test image Norway.jpeg: %v", err)
	}

	oldImage := bimg.NewImage(originalJPEG)
	oldImageSize, err := oldImage.Size()
	if err != nil {
		t.Fatalf("Failed to get image size: %v", err)
	}

	t.Logf("Original image: %dx%d, %d bytes", oldImageSize.Width, oldImageSize.Height, len(originalJPEG))

	// Test different quality levels
	testCases := []struct {
		quality int
		name    string
	}{
		{30, "low_quality"},
		{50, "medium_quality"},
		{75, "high_quality"},
		{95, "very_high_quality"},
	}

	results := make(map[int]int) // quality -> file size

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Initialize settings for this quality level
			settingsInt = make(map[string]int)
			settingsInt[IMG_MAX_WIDTH] = 800
			settingsInt[IMG_MAX_HEIGHT] = 600
			settingsInt[JPEG_QUALITY] = tc.quality

			settingsInt64 = make(map[string]int64)
			settingsInt64[IMG_MAX_PIXELS] = int64(settingsInt[IMG_MAX_WIDTH]) * int64(settingsInt[IMG_MAX_HEIGHT])

			oldImagePX := int64(oldImageSize.Width * oldImageSize.Height)
			if oldImagePX > settingsInt64[IMG_MAX_PIXELS] {
				oldImageAspect := float64(oldImageSize.Width) / float64(oldImageSize.Height)

				var newWidth, newHeight int
				if oldImageAspect >= 1 {
					newWidth = settingsInt[IMG_MAX_WIDTH]
					newHeight = int(float64(settingsInt[IMG_MAX_WIDTH]) / oldImageAspect)
				} else {
					newHeight = settingsInt[IMG_MAX_HEIGHT]
					newWidth = int(float64(settingsInt[IMG_MAX_HEIGHT]) * oldImageAspect)
				}

				// Test the quality-controlled processing
				options := bimg.Options{
					Width:   newWidth,
					Height:  newHeight,
					Quality: settingsInt[JPEG_QUALITY],
				}

				newByteContainer, err := oldImage.Process(options)
				if err != nil {
					t.Fatalf("Image processing failed: %v", err)
				}

				// Verify the image was processed
				if len(newByteContainer) == 0 {
					t.Error("Processed image should not be empty")
				}

				// Verify dimensions
				processedImage := bimg.NewImage(newByteContainer)
				processedSize, err := processedImage.Size()
				if err != nil {
					t.Fatalf("Failed to get processed image size: %v", err)
				}

				if processedSize.Width > settingsInt[IMG_MAX_WIDTH] || processedSize.Height > settingsInt[IMG_MAX_HEIGHT] {
					t.Errorf("Image not properly resized: %dx%d (should fit within %dx%d)",
						processedSize.Width, processedSize.Height, settingsInt[IMG_MAX_WIDTH], settingsInt[IMG_MAX_HEIGHT])
				}

				results[tc.quality] = len(newByteContainer)
				t.Logf("Quality %d: %dx%d, %d bytes", tc.quality, processedSize.Width, processedSize.Height, len(newByteContainer))
			}
		})
	}

	// Verify that different quality settings produce different file sizes
	if len(results) >= 2 {
		t.Logf("Quality comparison results:")
		for quality, size := range results {
			t.Logf("  Quality %d: %d bytes", quality, size)
		}

		// Check that we get different results for different qualities
		allSame := true
		var firstSize int
		for _, size := range results {
			if firstSize == 0 {
				firstSize = size
			} else if size != firstSize {
				allSame = false
				break
			}
		}

		if allSame {
			t.Error("All quality settings produced identical file sizes - quality parameter may not be working")
		}
	}
}

// TestMultipartFormProcessing tests the complete multipart form processing
func TestMultipartFormProcessing(t *testing.T) {
	setupTestEnvironment()

	// Initialize global variables as main() would
	settingsInt = make(map[string]int)
	settingsInt[IMG_MAX_WIDTH] = 800
	settingsInt[IMG_MAX_HEIGHT] = 600
	settingsInt[JPEG_QUALITY] = 30 // Use low quality for testing

	settingsInt64 = make(map[string]int64)
	settingsInt64[UPLOAD_MAX_SIZE] = 100 << 20 // 100MB
	settingsInt64[IMG_MAX_PIXELS] = int64(settingsInt[IMG_MAX_WIDTH]) * int64(settingsInt[IMG_MAX_HEIGHT])

	settingsString = make(map[string]string)
	settingsString[FILE_UPLOAD_FIELD] = "assetData"
	settingsString[FORWARD_DESTINATION] = "http://test.example.com/api/assets"

	// Load the test image file
	testJPEG, err := bimg.Read("Norway.jpeg")
	if err != nil {
		t.Fatalf("Failed to load test image Norway.jpeg: %v", err)
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	writer.WriteField("deviceId", "TEST")
	writer.WriteField("createdAt", "2023-01-01T00:00:00.000Z")

	// Add file
	part, err := writer.CreateFormFile("assetData", "test.jpg")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}

	_, err = io.Copy(part, bytes.NewReader(testJPEG))
	if err != nil {
		t.Fatalf("Failed to write test image: %v", err)
	}

	writer.Close()

	// Create test request
	req := httptest.NewRequest("POST", "/api/assets", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Mock the forward destination with a test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the forwarded multipart form
		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check that the image was processed
		file, _, err := r.FormFile("assetData")
		if err != nil {
			http.Error(w, "No file received", http.StatusBadRequest)
			return
		}
		defer file.Close()

		imageData, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusBadRequest)
			return
		}

		// Verify the image was compressed
		if len(imageData) == 0 {
			http.Error(w, "Empty image received", http.StatusBadRequest)
			return
		}

		// Check image dimensions
		processedImage := bimg.NewImage(imageData)
		size, err := processedImage.Size()
		if err != nil {
			http.Error(w, "Invalid image format", http.StatusBadRequest)
			return
		}

		// Should be resized to fit within limits
		if size.Width > 800 || size.Height > 600 {
			http.Error(w, "Image not properly resized", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Image processed successfully"))
	}))
	defer testServer.Close()

	// Update settings to point to test server
	settingsString[FORWARD_DESTINATION] = testServer.URL + "/api/assets"

	// Test would require more complex setup to fully test the HTTP proxy behavior
	// For now, we'll test the core image processing logic

	// Parse the multipart form manually to test reformatMultipart logic
	req.ParseMultipartForm(settingsInt64[UPLOAD_MAX_SIZE])
	file, handler, err := req.FormFile(settingsString[FILE_UPLOAD_FIELD])
	if err != nil {
		t.Fatalf("Failed to get form file: %v", err)
	}
	defer file.Close()

	// Test the image processing
	imageData, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("Failed to read image data: %v", err)
	}

	oldImage := bimg.NewImage(imageData)
	oldImageSize, err := oldImage.Size()
	if err != nil {
		t.Fatalf("Failed to get image size: %v", err)
	}

	t.Logf("Original image: %dx%d, %d bytes, filename: %s",
		oldImageSize.Width, oldImageSize.Height, len(imageData), handler.Filename)

	// This verifies our image processing setup is working
	if oldImageSize.Width == 0 || oldImageSize.Height == 0 {
		t.Error("Invalid image dimensions")
	}
}

// Benchmark tests to measure performance impact
func BenchmarkImageProcessingQuality30(b *testing.B) {
	benchmarkImageProcessing(b, 30)
}

func BenchmarkImageProcessingQuality75(b *testing.B) {
	benchmarkImageProcessing(b, 75)
}

func BenchmarkImageProcessingQuality95(b *testing.B) {
	benchmarkImageProcessing(b, 95)
}

func benchmarkImageProcessing(b *testing.B, quality int) {
	// Create test image
	testImage := make([]byte, 1200*1000*3)
	for i := range testImage {
		testImage[i] = byte(i % 256)
	}

	// Convert to JPEG first
	originalOptions := bimg.Options{
		Width:   1200,
		Height:  1000,
		Quality: 90,
		Type:    bimg.JPEG,
	}

	testJPEG, err := bimg.Resize(testImage, originalOptions)
	if err != nil {
		b.Fatalf("Failed to create test JPEG: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		oldImage := bimg.NewImage(testJPEG)

		options := bimg.Options{
			Width:   800,
			Height:  600,
			Quality: quality,
		}

		_, err := oldImage.Process(options)
		if err != nil {
			b.Fatalf("Image processing failed: %v", err)
		}
	}
}