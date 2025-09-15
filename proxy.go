package main

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/bimg"
)

// EXIF orientation constants
const (
	EXIF_ORIENTATION_NORMAL = 1
)

// Default MIME type constants
const (
	DEFAULT_MIME_TYPE = "application/octet-stream"
	JPEG_MIME_TYPE    = "image/jpeg"
	WEBP_MIME_TYPE    = "image/webp"
)

// Default settings constants
const (
	DEFAULT_IMG_MAX_WIDTH         = 1920
	DEFAULT_IMG_MAX_HEIGHT        = 1080
	DEFAULT_IMG_MAX_NARROW_SIDE   = 0  // 0 means not set, use original logic
	DEFAULT_JPEG_QUALITY          = 90 // Increased from 75 to 90 per author feedback
	DEFAULT_WEBP_QUALITY          = 85 // WebP default quality
	DEFAULT_NORMALIZE_EXTENSIONS  = 1  // 1 means normalize extensions, 0 means keep original
	DEFAULT_CONVERT_TO_FORMAT     = "" // Empty = disabled (backwards compatible)
)

// Image processing types
type ImageSize struct {
	Width  int
	Height int
}

type ImageProcessingSettings struct {
	MaxWidth      int
	MaxHeight     int
	MaxNarrowSide int
	JpegQuality   int
}

type ImageProcessingResult struct {
	ProcessedData   []byte
	WasCompressed   bool
	NewDimensions   ImageSize
	ProcessingError error
}

// Settings / Environment Variables
const IMG_MAX_WIDTH = "IMG_MAX_WIDTH"
const IMG_MAX_HEIGHT = "IMG_MAX_HEIGHT"
const IMG_MAX_NARROW_SIDE = "IMG_MAX_NARROW_SIDE"
const JPEG_QUALITY = "JPEG_QUALITY"
const WEBP_QUALITY = "WEBP_QUALITY"
const NORMALIZE_EXTENSIONS = "NORMALIZE_EXTENSIONS"

var intKeys = []string{IMG_MAX_WIDTH, IMG_MAX_HEIGHT, IMG_MAX_NARROW_SIDE, JPEG_QUALITY, WEBP_QUALITY, NORMALIZE_EXTENSIONS}
var settingsInt map[string]int

const UPLOAD_MAX_SIZE = "UPLOAD_MAX_SIZE"
const IMG_MAX_PIXELS = "IMG_MAX_PIXELS"

var int64Keys = []string{UPLOAD_MAX_SIZE, IMG_MAX_PIXELS}

var settingsInt64 map[string]int64

const FORWARD_DESTINATION = "FORWARD_DESTINATION"
const FILE_UPLOAD_FIELD = "FILE_UPLOAD_FIELD"
const LISTEN_PATH = "LISTEN_PATH"
const CONVERT_TO_FORMAT = "CONVERT_TO_FORMAT"

var stringKeys = []string{FORWARD_DESTINATION, FILE_UPLOAD_FIELD, LISTEN_PATH, CONVERT_TO_FORMAT}
var settingsString map[string]string

var client *http.Client

// initializeSettings initializes global settings from environment variables with validation
func initializeSettings() {
	// Integer32
	settingsInt = make(map[string]int)
	var defaultSettingsInt = map[string]int{
		IMG_MAX_WIDTH:         DEFAULT_IMG_MAX_WIDTH,
		IMG_MAX_HEIGHT:        DEFAULT_IMG_MAX_HEIGHT,
		IMG_MAX_NARROW_SIDE:   DEFAULT_IMG_MAX_NARROW_SIDE,
		JPEG_QUALITY:          DEFAULT_JPEG_QUALITY,
		WEBP_QUALITY:          DEFAULT_WEBP_QUALITY,
		NORMALIZE_EXTENSIONS:  DEFAULT_NORMALIZE_EXTENSIONS,
	}
	for _, intKey := range intKeys {
		settingsInt[intKey] = defaultSettingsInt[intKey]

		envValue := os.Getenv(intKey)
		if len(envValue) > 0 {
			convEnvValue, err := strconv.Atoi(envValue)
			if err == nil {
				// Validate environment variable values
				switch intKey {
				case JPEG_QUALITY, WEBP_QUALITY:
					if convEnvValue >= 1 && convEnvValue <= 100 {
						settingsInt[intKey] = convEnvValue
					} else {
						log.Printf("Invalid %s value %d, using default %d (valid range: 1-100)",
							intKey, convEnvValue, defaultSettingsInt[intKey])
					}
				case NORMALIZE_EXTENSIONS:
					if convEnvValue == 0 || convEnvValue == 1 {
						settingsInt[intKey] = convEnvValue
					} else {
						log.Printf("Invalid %s value %d, using default %d (valid values: 0 or 1)",
							intKey, convEnvValue, defaultSettingsInt[intKey])
					}
				default:
					// Other integer settings don't need special validation
					settingsInt[intKey] = convEnvValue
				}
			} else {
				log.Printf("Invalid %s value %q, using default %d",
					intKey, envValue, defaultSettingsInt[intKey])
			}
		}

		log.Println(intKey+": ", settingsInt[intKey])
	}

	// Integer64
	settingsInt64 = make(map[string]int64)
	var defaultSettingsInt64 = map[string]int64{
		UPLOAD_MAX_SIZE: 100 << 20,
		IMG_MAX_PIXELS:  int64(settingsInt[IMG_MAX_WIDTH]) * int64(settingsInt[IMG_MAX_HEIGHT]),
	}
	for _, int64Key := range int64Keys {
		settingsInt64[int64Key] = defaultSettingsInt64[int64Key]

		envValue := os.Getenv(int64Key)
		if len(envValue) > 0 {
			convEnvValue, err := strconv.ParseInt(envValue, 10, 64)
			if err == nil {
				settingsInt64[int64Key] = convEnvValue
			}
		}

		log.Println(int64Key+": ", settingsInt64[int64Key])
	}

	// Strings
	settingsString = make(map[string]string)
	var defaultSettingsString = map[string]string{
		FORWARD_DESTINATION: "https://httpbin.org/anything",
		FILE_UPLOAD_FIELD:   "assetData",
		LISTEN_PATH:         "/api/assets",
		CONVERT_TO_FORMAT:   DEFAULT_CONVERT_TO_FORMAT,
	}
	for _, stringKey := range stringKeys {
		settingsString[stringKey] = defaultSettingsString[stringKey]

		envValue := os.Getenv(stringKey)
		if len(envValue) > 0 {
			// Validate CONVERT_TO_FORMAT values
			if stringKey == CONVERT_TO_FORMAT {
				normalizedFormat := strings.ToUpper(strings.TrimSpace(envValue))
				if normalizedFormat == "" || normalizedFormat == "JPEG" || normalizedFormat == "WEBP" {
					settingsString[stringKey] = normalizedFormat
				} else {
					log.Printf("Invalid %s value %q, using default %q (valid values: \"\", \"JPEG\", \"WEBP\")",
						stringKey, envValue, defaultSettingsString[stringKey])
				}
			} else {
				settingsString[stringKey] = envValue
			}
		}

		log.Println(stringKey+": ", settingsString[stringKey])
	}
}

/*
Test with
curl --header "X-Test: hello" -F "deviceAssetId=web-input.jpg-1672571948584" -F "deviceId=WEB" -F "createdAt=2016-12-02T10:10:20.000Z" -F "modifiedAt=2023-01-01T11:19:08.584Z" -F "isFavorite=false" -F "duration=0:00:00.000000" -F "fileExtension=.jpg" -F "assetData=@example.jpg" http://localhost:6743/upload
*/
func main() {
	initializeSettings()

	// Other
	client = &http.Client{
		Timeout: time.Second * 60,  // Increased from 10s to 60s for large files (videos, high-res images)
	}

	http.HandleFunc(settingsString[LISTEN_PATH], proxyHandler)
	if settingsString[LISTEN_PATH] != "/" {
		http.HandleFunc("/", proxyHandler)
	}
	http.ListenAndServe(":6743", nil)
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	body := &bytes.Buffer{}
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		log.Println("Incoming file upload")

		var err error
		contentType, body, err = reformatMultipart(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		byteBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body = bytes.NewBuffer(byteBody)
	}

	// Forward request
	proxyReq, _ := http.NewRequest(r.Method, settingsString[FORWARD_DESTINATION], body)
	copyHeader(proxyReq.Header, r.Header)
	proxyReq.Header.Set("Content-Length", strconv.Itoa(body.Len()))
	proxyReq.Header.Set("Content-Type", contentType)

	if r.URL.Path != settingsString[LISTEN_PATH] {
		log.Println("Request hit proxy but not the intended path, proxying to copied path")
		proxyReq.URL.Path = r.URL.Path
		proxyReq.URL.RawQuery = r.URL.RawQuery
	}

	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		log.Println("ProxyResp Error:", err)
		http.Error(w, err.Error(), http.StatusFailedDependency)
		return
	}

	// Send result back
	copyHeader(w.Header(), proxyResp.Header)
	w.WriteHeader(proxyResp.StatusCode)
	io.Copy(w, proxyResp.Body)
}

func reformatMultipart(w http.ResponseWriter, r *http.Request) (string, *bytes.Buffer, error) {
	r.ParseMultipartForm(settingsInt64[UPLOAD_MAX_SIZE])
	file, handler, err := r.FormFile(settingsString[FILE_UPLOAD_FIELD])
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	// Read and parse image
	byteContainer, err := io.ReadAll(file)
	if err != nil {
		log.Printf("Failed to read file: %v", err)
		return "", nil, err
	}

	// Use the helper function to process the image
	settings := ImageProcessingSettings{
		MaxWidth:      settingsInt[IMG_MAX_WIDTH],
		MaxHeight:     settingsInt[IMG_MAX_HEIGHT],
		MaxNarrowSide: settingsInt[IMG_MAX_NARROW_SIDE],
		JpegQuality:   settingsInt[JPEG_QUALITY],
	}

	result, err := processImageWithStrategy(byteContainer, settings)

	// Track processing results
	var wasImageProcessed bool
	var actuallyCompressed bool

	if err == nil {
		wasImageProcessed = true
		actuallyCompressed = result.WasCompressed
		byteContainer = result.ProcessedData
	} else {
		log.Printf("Image processing error: %v", err)
		wasImageProcessed = false
		actuallyCompressed = false
		// byteContainer remains original data
	}

	// Copy form values
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for formKey := range r.Form {
		formValue := r.Form.Get(formKey)
		// fmt.Println(formKey, " => ", formValue)

		fw, _ := writer.CreateFormField(formKey)
		io.Copy(fw, strings.NewReader(formValue))
	}

	// Add new file with proper filename and MIME type
	var finalFilename string
	var finalMimeType string

	// Determine target format and MIME type
	convertFormat := settingsString[CONVERT_TO_FORMAT]

	if wasImageProcessed && actuallyCompressed {
		// Image was converted to a new format
		switch convertFormat {
		case "JPEG":
			finalMimeType = JPEG_MIME_TYPE
			if settingsInt[NORMALIZE_EXTENSIONS] == 1 {
				finalFilename = changeExtensionToJPG(handler.Filename)
				log.Printf("Converted to JPEG with normalized filename: %s -> %s", handler.Filename, finalFilename)
			} else {
				finalFilename = handler.Filename
				log.Printf("Converted to JPEG but keeping original filename: %s", finalFilename)
			}
		case "WEBP":
			finalMimeType = WEBP_MIME_TYPE
			if settingsInt[NORMALIZE_EXTENSIONS] == 1 {
				finalFilename = changeExtensionToWebP(handler.Filename)
				log.Printf("Converted to WebP with normalized filename: %s -> %s", handler.Filename, finalFilename)
			} else {
				finalFilename = handler.Filename
				log.Printf("Converted to WebP but keeping original filename: %s", finalFilename)
			}
		default:
			// Fallback (shouldn't happen)
			finalMimeType = JPEG_MIME_TYPE
			finalFilename = handler.Filename
			log.Printf("Unknown convert format, defaulting to JPEG MIME: %s", finalFilename)
		}
	} else if wasImageProcessed && !actuallyCompressed {
		// Image was processed but original format was kept (compression didn't help or conversion disabled)
		finalFilename = handler.Filename
		finalMimeType = handler.Header.Get("Content-Type")
		if finalMimeType == "" {
			finalMimeType = DEFAULT_MIME_TYPE
		}
		if convertFormat == "" {
			log.Printf("Image resized but format conversion disabled: %s (%s)", finalFilename, finalMimeType)
		} else {
			log.Printf("Image processed but original kept (better compression): %s (%s)", finalFilename, finalMimeType)
		}
	} else {
		// Not an image or processing failed - keep everything original
		finalFilename = handler.Filename
		finalMimeType = handler.Header.Get("Content-Type")
		if finalMimeType == "" {
			finalMimeType = DEFAULT_MIME_TYPE
		}
		log.Printf("Non-image file or processing failed, keeping original: %s (%s)", finalFilename, finalMimeType)
	}

	fw, _ := CreateFormFileWithMime(writer, settingsString[FILE_UPLOAD_FIELD], finalFilename, finalMimeType)
	io.Copy(fw, bytes.NewReader(byteContainer))
	writer.Close()

	contentType := writer.FormDataContentType()

	return contentType, body, nil
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// changeExtensionToJPG changes the file extension to .JPG
// This function is only called for valid image files that have been successfully processed
func changeExtensionToJPG(filename string) string {
	extension := filepath.Ext(filename)

	// Handle files with no extension
	if extension == "" {
		return filename + ".JPG"
	}

	// Handle files with extension - replace the extension
	nameWithoutExt := strings.TrimSuffix(filename, extension)
	return nameWithoutExt + ".JPG"
}

// changeExtensionToWebP changes the file extension to .WEBP
// This function is only called for valid image files that have been successfully processed
func changeExtensionToWebP(filename string) string {
	extension := filepath.Ext(filename)

	// Handle files with no extension
	if extension == "" {
		return filename + ".WEBP"
	}

	// Handle files with extension - replace the extension
	nameWithoutExt := strings.TrimSuffix(filename, extension)
	return nameWithoutExt + ".WEBP"
}

func CreateFormFileWithMime(w *multipart.Writer, fieldname, filename, mimeType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+escapeQuotes(fieldname)+`"; filename="`+escapeQuotes(filename)+`"`)
	h.Set("Content-Type", mimeType)
	return w.CreatePart(h)
}

var ignoreHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Accept-Encoding",
	"Host",
	"Cf-Ipcountry",
	"Cf-Connecting-Ip",
	"X-Forwarded-Proto",
	"X-Forwarded-For",
	"Cf-Ray",
	"Cf-Visitor",
	"Cf-Warp-Tag-Id",
	"Content-Type",
	"Origin",
	"X-Amzn-Trace-Id",
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		ignoreThisHeader := false
		for _, ignoreHeader := range ignoreHeaders {
			if strings.ToLower(k) == strings.ToLower(ignoreHeader) {
				ignoreThisHeader = true
				break
			}
		}
		if ignoreThisHeader {
			continue
		}

		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// detectImageTransparency checks if an image has an alpha channel using bimg metadata
func detectImageTransparency(imageData []byte) (bool, error) {
	image := bimg.NewImage(imageData)
	metadata, err := image.Metadata()
	if err != nil {
		return false, err
	}
	return metadata.Alpha, nil
}

// processImageWithStrategy processes an image according to the given settings
func processImageWithStrategy(originalData []byte, settings ImageProcessingSettings) (*ImageProcessingResult, error) {
	// Early transparency detection - skip any format conversion if image has transparency
	convertFormat := settingsString[CONVERT_TO_FORMAT]
	if convertFormat != "" {
		hasTransparency, err := detectImageTransparency(originalData)
		if err == nil && hasTransparency {
			log.Printf("Skipping %s conversion - image has transparency", convertFormat)
			return &ImageProcessingResult{
				ProcessedData:   originalData,
				WasCompressed:   false,
				ProcessingError: nil,
			}, nil
		}
	}

	// Handle EXIF orientation first - this gives us the corrected image and bytes
	workingImage, rotatedData, err := handleEXIFOrientation(originalData)
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   rotatedData,  // rotatedData will be originalData if rotation failed
			WasCompressed:   false,
			ProcessingError: err,
		}, err
	}

	// Get size from properly oriented image
	oldImageSize, err := workingImage.Size()
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   rotatedData,  // Return rotated data even if processing fails
			WasCompressed:   false,
			ProcessingError: err,
		}, err
	}

	// Calculate resize dimensions
	newDimensions := calculateResizeDimensions(
		ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
		settings,
	)

	// Check if format conversion is enabled (reuse convertFormat from earlier)
	if convertFormat == "" {
		// No format conversion - just resize if needed (backwards compatible behavior)
		needsResize := newDimensions.Width != oldImageSize.Width || newDimensions.Height != oldImageSize.Height
		if !needsResize {
			// No processing needed
			return &ImageProcessingResult{
				ProcessedData:   rotatedData,
				WasCompressed:   false,
				NewDimensions:   ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
				ProcessingError: nil,
			}, nil
		}

		// Resize only (preserve original format)
		options := bimg.Options{
			Width:  newDimensions.Width,
			Height: newDimensions.Height,
		}

		processedData, err := workingImage.Process(options)
		if err != nil {
			return &ImageProcessingResult{
				ProcessedData:   rotatedData,
				WasCompressed:   false,
				NewDimensions:   ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
				ProcessingError: err,
			}, err
		}

		return &ImageProcessingResult{
			ProcessedData: processedData,
			WasCompressed: false, // We didn't change format, just resized
			NewDimensions: newDimensions,
		}, nil
	}

	// Format conversion is enabled - process the image with format conversion
	var targetType bimg.ImageType
	var quality int

	switch convertFormat {
	case "JPEG":
		targetType = bimg.JPEG
		quality = settings.JpegQuality
	case "WEBP":
		targetType = bimg.WEBP
		quality = settingsInt[WEBP_QUALITY]
	default:
		// Shouldn't happen due to validation, but fallback to JPEG
		targetType = bimg.JPEG
		quality = settings.JpegQuality
	}

	options := bimg.Options{
		Width:   newDimensions.Width,
		Height:  newDimensions.Height,
		Quality: quality,
		Type:    targetType,
	}

	// Note: WebP transparency fallback is now handled in early detection phase

	processedData, err := workingImage.Process(options)
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   rotatedData,  // Return rotated data even if processing fails
			WasCompressed:   false,
			NewDimensions:   ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
			ProcessingError: err,
		}, err
	}

	// Only use converted data if it's actually smaller (the whole point of conversion is optimization)
	wasCompressed := len(processedData) < len(rotatedData)
	
	var finalData []byte
	if wasCompressed {
		finalData = processedData
		log.Printf("Conversion to %s successful: %d → %d bytes", convertFormat, len(rotatedData), len(processedData))
	} else {
		finalData = rotatedData  // Use rotated data (preserves EXIF rotation)
		log.Printf("Conversion to %s skipped - would increase size: %d → %d bytes", convertFormat, len(rotatedData), len(processedData))
	}

	return &ImageProcessingResult{
		ProcessedData: finalData,
		WasCompressed: wasCompressed,
		NewDimensions: newDimensions,
	}, nil
}

// handleEXIFOrientation handles EXIF orientation correction
// Returns both the corrected image and the corrected bytes
func handleEXIFOrientation(originalData []byte) (*bimg.Image, []byte, error) {
	image := bimg.NewImage(originalData)
	metadata, err := image.Metadata()
	needsRotation := err == nil && metadata.Orientation > EXIF_ORIENTATION_NORMAL

	if needsRotation {
		log.Println("EXIF orientation detected, applying rotation")
		rotatedBytes, err := image.AutoRotate()
		if err != nil {
			log.Printf("AutoRotate failed, using original orientation: %v", err)
			return image, originalData, nil // Return original, not an error
		}
		return bimg.NewImage(rotatedBytes), rotatedBytes, nil
	}

	return image, originalData, nil
}

// calculateResizeDimensions calculates the new dimensions based on resize strategy
func calculateResizeDimensions(original ImageSize, settings ImageProcessingSettings) ImageSize {
	if settings.MaxNarrowSide > 0 {
		return calculateNarrowSideResize(original, settings.MaxNarrowSide)
	}
	return calculateBoundingBoxResize(original, settings.MaxWidth, settings.MaxHeight)
}

// calculateNarrowSideResize implements narrow side constraint strategy
func calculateNarrowSideResize(original ImageSize, maxNarrowSide int) ImageSize {
	narrowSide := original.Width
	if original.Height < original.Width {
		narrowSide = original.Height
	}

	if narrowSide <= maxNarrowSide {
		return original // No resize needed
	}

	scale := float64(maxNarrowSide) / float64(narrowSide)
	return ImageSize{
		Width:  int(float64(original.Width) * scale),
		Height: int(float64(original.Height) * scale),
	}
}

// calculateBoundingBoxResize implements traditional bounding box strategy
func calculateBoundingBoxResize(original ImageSize, maxWidth, maxHeight int) ImageSize {
	if original.Width <= maxWidth && original.Height <= maxHeight {
		return original // No resize needed
	}

	scaleWidth := float64(maxWidth) / float64(original.Width)
	scaleHeight := float64(maxHeight) / float64(original.Height)

	// Use the smaller scale factor to ensure both dimensions fit
	scale := scaleWidth
	if scaleHeight < scaleWidth {
		scale = scaleHeight
	}

	return ImageSize{
		Width:  int(float64(original.Width) * scale),
		Height: int(float64(original.Height) * scale),
	}
}
