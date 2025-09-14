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
const NORMALIZE_EXTENSIONS = "NORMALIZE_EXTENSIONS"

var intKeys = []string{IMG_MAX_WIDTH, IMG_MAX_HEIGHT, IMG_MAX_NARROW_SIDE, JPEG_QUALITY, NORMALIZE_EXTENSIONS}
var settingsInt map[string]int

const UPLOAD_MAX_SIZE = "UPLOAD_MAX_SIZE"
const IMG_MAX_PIXELS = "IMG_MAX_PIXELS"

var int64Keys = []string{UPLOAD_MAX_SIZE, IMG_MAX_PIXELS}

var settingsInt64 map[string]int64

const FORWARD_DESTINATION = "FORWARD_DESTINATION"
const FILE_UPLOAD_FIELD = "FILE_UPLOAD_FIELD"
const LISTEN_PATH = "LISTEN_PATH"

var stringKeys = []string{FORWARD_DESTINATION, FILE_UPLOAD_FIELD, LISTEN_PATH}
var settingsString map[string]string

var client *http.Client

// initializeSettings initializes global settings from environment variables with validation
func initializeSettings() {
	// Integer32
	settingsInt = make(map[string]int)
	var defaultSettingsInt = map[string]int{
		IMG_MAX_WIDTH:         1920,
		IMG_MAX_HEIGHT:        1080,
		IMG_MAX_NARROW_SIDE:   0, // 0 means not set, use original logic
		JPEG_QUALITY:          75,
		NORMALIZE_EXTENSIONS:  1, // 1 means normalize to .jpg, 0 means keep original
	}
	var intKeys = []string{IMG_MAX_WIDTH, IMG_MAX_HEIGHT, IMG_MAX_NARROW_SIDE, JPEG_QUALITY, NORMALIZE_EXTENSIONS}
	for _, intKey := range intKeys {
		settingsInt[intKey] = defaultSettingsInt[intKey]

		envValue := os.Getenv(intKey)
		if len(envValue) > 0 {
			convEnvValue, err := strconv.Atoi(envValue)
			if err == nil {
				// Validate environment variable values
				switch intKey {
				case JPEG_QUALITY:
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
	}
	for _, stringKey := range stringKeys {
		settingsString[stringKey] = defaultSettingsString[stringKey]

		envValue := os.Getenv(stringKey)
		if len(envValue) > 0 {
			settingsString[stringKey] = envValue
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
		Timeout: time.Second * 10,
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
	oldImage := bimg.NewImage(byteContainer)
	
	// Check if image has EXIF orientation data that needs correction
	var workingImage *bimg.Image
	
	// Get EXIF orientation (this is fast, just metadata reading)
	metadata, metadataErr := oldImage.Metadata()
	needsRotation := metadataErr == nil && metadata.Orientation > EXIF_ORIENTATION_NORMAL
	
	if needsRotation {
		// Only do expensive AutoRotate when actually needed
		log.Println("EXIF orientation detected, applying rotation")
		rotatedImage, rotateErr := oldImage.AutoRotate()
		if rotateErr != nil {
			log.Printf("AutoRotate failed, using original orientation: %v", rotateErr)
			workingImage = oldImage
		} else {
			workingImage = bimg.NewImage(rotatedImage)
			// CRITICAL FIX: Update byteContainer with rotated image when no further processing needed
			byteContainer = rotatedImage
		}
	} else {
		// No rotation needed, use original image directly (fast path)
		workingImage = oldImage
	}
	
	// Get size from properly oriented image
	oldImageSize, sizeErr := workingImage.Size()
	
	// Track processing results with clear variables
	var wasImageProcessed bool  // True if we successfully parsed as image
	var actuallyCompressed bool // True if we replaced bytes with compressed version
	
	// Process image only if we got valid size information
	if sizeErr == nil {
		wasImageProcessed = true
		
		var newWidth, newHeight int
		var needsResize bool
		
		// Check if narrow side constraint is set (priority over width/height limits)
		narrowSideLimit, narrowSideSet := settingsInt[IMG_MAX_NARROW_SIDE]
		if narrowSideSet && narrowSideLimit > 0 {
			// Use narrow side strategy
			narrowSide := oldImageSize.Width
			if oldImageSize.Height < oldImageSize.Width {
				narrowSide = oldImageSize.Height
			}
			
			needsResize = narrowSide > narrowSideLimit
			
			if needsResize {
				log.Println("Resizing needed - narrow side exceeds limit")
				
				// Calculate scale factor based on narrow side
				scale := float64(narrowSideLimit) / float64(narrowSide)
				
				newWidth = int(float64(oldImageSize.Width) * scale)
				newHeight = int(float64(oldImageSize.Height) * scale)
			} else {
				log.Println("No resizing needed - narrow side within limit")
				newWidth = oldImageSize.Width
				newHeight = oldImageSize.Height
			}
		} else {
			// Use original bounding box strategy
			needsResize = oldImageSize.Width > settingsInt[IMG_MAX_WIDTH] || oldImageSize.Height > settingsInt[IMG_MAX_HEIGHT]
			
			if needsResize {
				log.Println("Resizing needed - image exceeds limits")
				
				// Calculate scale factors for both dimensions
				scaleWidth := float64(settingsInt[IMG_MAX_WIDTH]) / float64(oldImageSize.Width)
				scaleHeight := float64(settingsInt[IMG_MAX_HEIGHT]) / float64(oldImageSize.Height)
				
				// Use the smaller scale factor to ensure both dimensions fit
				scale := scaleWidth
				if scaleHeight < scaleWidth {
					scale = scaleHeight
				}
				
				newWidth = int(float64(oldImageSize.Width) * scale)
				newHeight = int(float64(oldImageSize.Height) * scale)
			} else {
				log.Println("No resizing needed - keeping original dimensions")
				newWidth = oldImageSize.Width
				newHeight = oldImageSize.Height
			}
		}

		// Always process the image (for format conversion and quality compression)
		options := bimg.Options{
			Width:   newWidth,
			Height:  newHeight,
			Quality: settingsInt[JPEG_QUALITY],
			Type:    bimg.JPEG,
		}
		
		newByteContainer, processErr := workingImage.Process(options)
		if processErr == nil {
			if len(byteContainer) > len(newByteContainer) {
				log.Println("Processing saved space, so we're taking that")
				byteContainer = newByteContainer
				actuallyCompressed = true
			} else {
				log.Println("After processing, original file is smaller - therefore keeping the original")
				actuallyCompressed = false
			}
		} else {
			log.Printf("Processing error: %v", processErr)
			actuallyCompressed = false
		}
	} else {
		log.Printf("Size() Error: %v", sizeErr)
		wasImageProcessed = false
		actuallyCompressed = false
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
	
	// CRITICAL FIX: Use the correct wasImageProcessed variable we set above
	// instead of relying on unclear 'err' variable
	
	if wasImageProcessed && actuallyCompressed && settingsInt[NORMALIZE_EXTENSIONS] == 1 {
		// Only normalize extensions for images that were actually compressed to JPEG
		finalMimeType = JPEG_MIME_TYPE
		finalFilename = changeExtensionToJPG(handler.Filename)
		log.Printf("Normalizing compressed image filename: %s -> %s", handler.Filename, finalFilename)
	} else if wasImageProcessed && actuallyCompressed {
		// Image was compressed but keep original filename
		finalFilename = handler.Filename
		finalMimeType = JPEG_MIME_TYPE  // Fix MIME type since content is JPEG
		log.Printf("Image compressed to JPEG but keeping filename: %s", finalFilename)
	} else if wasImageProcessed && !actuallyCompressed {
		// Image was processed but original was kept (compression didn't help)
		finalFilename = handler.Filename
		finalMimeType = handler.Header.Get("Content-Type")
		if finalMimeType == "" {
			finalMimeType = DEFAULT_MIME_TYPE
		}
		log.Printf("Image processed but original kept (better compression): %s (%s)", finalFilename, finalMimeType)
	} else {
		// Not an image or processing failed - keep everything original
		finalFilename = handler.Filename
		finalMimeType = handler.Header.Get("Content-Type")
		if finalMimeType == "" {
			finalMimeType = DEFAULT_MIME_TYPE
		}
		log.Printf("Non-image file, keeping original: %s (%s)", finalFilename, finalMimeType)
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

// changeExtensionToJPG changes the file extension to .jpg
// This function is only called for valid image files that have been successfully processed
func changeExtensionToJPG(filename string) string {
	extension := filepath.Ext(filename)
	
	// Handle files with no extension 
	if extension == "" {
		return filename + ".jpg"
	}
	
	// Handle files with extension - replace the extension
	nameWithoutExt := strings.TrimSuffix(filename, extension)
	return nameWithoutExt + ".jpg"
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

// processImageWithStrategy processes an image according to the given settings
func processImageWithStrategy(originalData []byte, settings ImageProcessingSettings) (*ImageProcessingResult, error) {
	oldImage := bimg.NewImage(originalData)
	
	// Handle EXIF orientation first
	workingImage, err := handleEXIFOrientation(oldImage)
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   originalData,
			WasCompressed:   false,
			ProcessingError: err,
		}, err
	}
	
	// Get size from properly oriented image
	oldImageSize, err := workingImage.Size()
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   originalData,
			WasCompressed:   false,
			ProcessingError: err,
		}, err
	}
	
	// Calculate resize dimensions
	newDimensions := calculateResizeDimensions(
		ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
		settings,
	)
	
	// Process the image
	options := bimg.Options{
		Width:   newDimensions.Width,
		Height:  newDimensions.Height,
		Quality: settings.JpegQuality,
		Type:    bimg.JPEG,
	}
	
	processedData, err := workingImage.Process(options)
	if err != nil {
		return &ImageProcessingResult{
			ProcessedData:   originalData,
			WasCompressed:   false,
			NewDimensions:   ImageSize{Width: oldImageSize.Width, Height: oldImageSize.Height},
			ProcessingError: err,
		}, err
	}
	
	// Determine if compression was beneficial
	wasCompressed := len(processedData) < len(originalData)
	finalData := originalData
	if wasCompressed {
		finalData = processedData
	}
	
	return &ImageProcessingResult{
		ProcessedData: finalData,
		WasCompressed: wasCompressed,
		NewDimensions: newDimensions,
	}, nil
}

// handleEXIFOrientation handles EXIF orientation correction
func handleEXIFOrientation(image *bimg.Image) (*bimg.Image, error) {
	metadata, err := image.Metadata()
	needsRotation := err == nil && metadata.Orientation > EXIF_ORIENTATION_NORMAL
	
	if needsRotation {
		log.Println("EXIF orientation detected, applying rotation")
		rotatedImage, err := image.AutoRotate()
		if err != nil {
			log.Printf("AutoRotate failed, using original orientation: %v", err)
			return image, nil // Return original, not an error
		}
		return bimg.NewImage(rotatedImage), nil
	}
	
	return image, nil
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
