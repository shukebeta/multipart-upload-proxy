package main

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
)

// reformatMultipart processes a multipart form request, extracts and processes the image file,
// and returns a new multipart form with the processed image and all other form fields.
// Maintains original signature for backward compatibility with existing tests.
func reformatMultipart(w http.ResponseWriter, r *http.Request, cfg *Config) (string, *bytes.Buffer, error) {
	r.ParseMultipartForm(cfg.UploadMaxSize)
	file, handler, err := r.FormFile(cfg.FileUploadField)
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
		MaxWidth:        cfg.ImgMaxWidth,
		MaxHeight:       cfg.ImgMaxHeight,
		MaxNarrowSide:   cfg.ImgMaxNarrowSide,
		JpegQuality:     cfg.JpegQuality,
		WebpQuality:     cfg.WebpQuality,
		ConvertToFormat: cfg.ConvertToFormat,
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
	convertFormat := cfg.ConvertToFormat

	if wasImageProcessed && actuallyCompressed {
		// Image was converted to a new format
		switch convertFormat {
		case "JPEG":
			finalMimeType = JPEG_MIME_TYPE
			if cfg.NormalizeExt {
				finalFilename = changeExtensionToJPG(handler.Filename)
				log.Printf("Converted to JPEG with normalized filename: %s -> %s", handler.Filename, finalFilename)
			} else {
				finalFilename = handler.Filename
				log.Printf("Converted to JPEG but keeping original filename: %s", finalFilename)
			}
		case "WEBP":
			finalMimeType = WEBP_MIME_TYPE
			if cfg.NormalizeExt {
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

	fw, _ := CreateFormFileWithMime(writer, cfg.FileUploadField, finalFilename, finalMimeType)
	io.Copy(fw, bytes.NewReader(byteContainer))
	writer.Close()

	contentType := writer.FormDataContentType()

	return contentType, body, nil
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// escapeQuotes escapes quotes and backslashes in a string for use in multipart form headers
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

// CreateFormFileWithMime creates a new multipart form file field with a custom MIME type
func CreateFormFileWithMime(w *multipart.Writer, fieldname, filename, mimeType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+escapeQuotes(fieldname)+`"; filename="`+escapeQuotes(filename)+`"`)
	h.Set("Content-Type", mimeType)
	return w.CreatePart(h)
}