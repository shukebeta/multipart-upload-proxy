package main

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration parsed from environment variables
type Config struct {
	ImgMaxWidth        int
	ImgMaxHeight       int
	ImgMaxNarrowSide   int
	JpegQuality        int
	WebpQuality        int
	NormalizeExt       bool
	UploadMaxSize      int64
	ImgMaxPixels       int64
	ForwardDestination string
	FileUploadField    string
	ListenPath         string
	ConvertToFormat    string
}

// NewConfigFromEnv parses env vars, validates and returns a Config.
func NewConfigFromEnv() *Config {
	cfg := &Config{
		ImgMaxWidth:        DEFAULT_IMG_MAX_WIDTH,
		ImgMaxHeight:       DEFAULT_IMG_MAX_HEIGHT,
		ImgMaxNarrowSide:   DEFAULT_IMG_MAX_NARROW_SIDE,
		JpegQuality:        DEFAULT_JPEG_QUALITY,
		WebpQuality:        DEFAULT_WEBP_QUALITY,
		NormalizeExt:       DEFAULT_NORMALIZE_EXTENSIONS == 1,
		UploadMaxSize:      100 << 20, // 100MB default
		ForwardDestination: "https://httpbin.org/anything",
		FileUploadField:    "assetData",
		ListenPath:         "/api/assets",
		ConvertToFormat:    DEFAULT_CONVERT_TO_FORMAT,
	}

	// Parse integer settings with validation
	if v := os.Getenv(IMG_MAX_WIDTH); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ImgMaxWidth = n
		} else {
			log.Printf("Invalid %s=%q, using %d", IMG_MAX_WIDTH, v, cfg.ImgMaxWidth)
		}
	}

	if v := os.Getenv(IMG_MAX_HEIGHT); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ImgMaxHeight = n
		} else {
			log.Printf("Invalid %s=%q, using %d", IMG_MAX_HEIGHT, v, cfg.ImgMaxHeight)
		}
	}

	if v := os.Getenv(IMG_MAX_NARROW_SIDE); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.ImgMaxNarrowSide = n
		} else {
			log.Printf("Invalid %s=%q, using %d", IMG_MAX_NARROW_SIDE, v, cfg.ImgMaxNarrowSide)
		}
	}

	if v := os.Getenv(JPEG_QUALITY); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 100 {
			cfg.JpegQuality = n
		} else {
			log.Printf("Invalid %s=%q, using %d", JPEG_QUALITY, v, cfg.JpegQuality)
		}
	}

	if v := os.Getenv(WEBP_QUALITY); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 100 {
			cfg.WebpQuality = n
		} else {
			log.Printf("Invalid %s=%q, using %d", WEBP_QUALITY, v, cfg.WebpQuality)
		}
	}

	if v := os.Getenv(NORMALIZE_EXTENSIONS); v != "" {
		if n, err := strconv.Atoi(v); err == nil && (n == 0 || n == 1) {
			cfg.NormalizeExt = (n == 1)
		} else {
			log.Printf("Invalid %s=%q, using %t", NORMALIZE_EXTENSIONS, v, cfg.NormalizeExt)
		}
	}

	// Parse int64 settings
	if v := os.Getenv(UPLOAD_MAX_SIZE); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.UploadMaxSize = n
		} else {
			log.Printf("Invalid %s=%q, using %d", UPLOAD_MAX_SIZE, v, cfg.UploadMaxSize)
		}
	}

	// Parse string settings
	if v := os.Getenv(FORWARD_DESTINATION); v != "" {
		cfg.ForwardDestination = v
	}

	if v := os.Getenv(FILE_UPLOAD_FIELD); v != "" {
		cfg.FileUploadField = v
	}

	if v := os.Getenv(LISTEN_PATH); v != "" {
		cfg.ListenPath = v
	}

	if v := os.Getenv(CONVERT_TO_FORMAT); v != "" {
		normalizedFormat := strings.ToUpper(strings.TrimSpace(v))
		if normalizedFormat == "" || normalizedFormat == "JPEG" || normalizedFormat == "WEBP" {
			cfg.ConvertToFormat = normalizedFormat
		} else {
			log.Printf("Invalid %s=%q, using %q (valid values: \"\", \"JPEG\", \"WEBP\")",
				CONVERT_TO_FORMAT, v, cfg.ConvertToFormat)
		}
	}

	// Compute derived values
	cfg.ImgMaxPixels = int64(cfg.ImgMaxWidth) * int64(cfg.ImgMaxHeight)


	return cfg
}


