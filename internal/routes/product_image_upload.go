package routes

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"

	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

const (
	maxProductImageInputBytes = 900 * 1024
	maxProductImageDimension  = 2048
	maxProductImagePixels     = 4_000_000
	productImageDecodeSlots   = 2
)

const invalidProductImageMessage = "Use a JPEG or PNG under 900 KB and no larger than 2048 × 2048 pixels; optimized images must be under 256 KB."

var productImageDecodeSemaphore = make(chan struct{}, productImageDecodeSlots)

func parseProductImageUpload(r *http.Request) (*partnersvc.ProductImageUpload, error) {
	headerContentType := r.Header.Get("Content-Type")
	contentType, _, err := mime.ParseMediaType(headerContentType)
	if err != nil {
		if strings.HasPrefix(strings.ToLower(headerContentType), "multipart/form-data") {
			return nil, partnersvc.ErrInvalidProductImage
		}
		return nil, nil
	}
	if !strings.EqualFold(contentType, "multipart/form-data") {
		return nil, nil
	}
	if err := r.ParseMultipartForm(32 << 10); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		return nil, partnersvc.ErrInvalidProductImage
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		return nil, partnersvc.ErrInvalidProductImage
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxProductImageInputBytes {
		return nil, partnersvc.ErrInvalidProductImage
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxProductImageInputBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxProductImageInputBytes {
		return nil, partnersvc.ErrInvalidProductImage
	}

	select {
	case productImageDecodeSemaphore <- struct{}{}:
		defer func() { <-productImageDecodeSemaphore }()
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 ||
		config.Width > maxProductImageDimension || config.Height > maxProductImageDimension ||
		int64(config.Width)*int64(config.Height) > maxProductImagePixels {
		return nil, partnersvc.ErrInvalidProductImage
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != format {
		return nil, partnersvc.ErrInvalidProductImage
	}

	var normalized bytes.Buffer
	storedType := "image/png"
	if format == "jpeg" {
		storedType = "image/jpeg"
		err = jpeg.Encode(&normalized, decoded, &jpeg.Options{Quality: 80})
	} else {
		err = png.Encode(&normalized, decoded)
	}
	if err != nil || normalized.Len() == 0 || normalized.Len() > partnersvc.MaxProductImageBytes {
		return nil, partnersvc.ErrInvalidProductImage
	}
	return &partnersvc.ProductImageUpload{ContentType: storedType, Data: normalized.Bytes()}, nil
}
