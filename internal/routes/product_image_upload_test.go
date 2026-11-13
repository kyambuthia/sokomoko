package routes

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

func TestProductMediaRequestLimit(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		host        string
		path        string
		contentType string
		wantLimit   int64
	}{
		{
			name:        "partner new product multipart post gets limit",
			method:      http.MethodPost,
			host:        "partner.localhost:6969",
			path:        "/products/new",
			contentType: "multipart/form-data; boundary=xyz",
			wantLimit:   maxProductMediaRequestBytes,
		},
		{
			name:        "partner edit product multipart post gets limit",
			method:      http.MethodPost,
			host:        "partner.localhost:6969",
			path:        "/products/123/edit",
			contentType: "multipart/form-data; boundary=xyz",
			wantLimit:   maxProductMediaRequestBytes,
		},
		{
			name:        "partner edit product with non-numeric id gets zero",
			method:      http.MethodPost,
			host:        "partner.localhost:6969",
			path:        "/products/abc/edit",
			contentType: "multipart/form-data; boundary=xyz",
			wantLimit:   0,
		},
		{
			name:        "non-partner host gets zero",
			method:      http.MethodPost,
			host:        "localhost:6969",
			path:        "/products/new",
			contentType: "multipart/form-data; boundary=xyz",
			wantLimit:   0,
		},
		{
			name:        "get request gets zero",
			method:      http.MethodGet,
			host:        "partner.localhost:6969",
			path:        "/products/new",
			contentType: "multipart/form-data; boundary=xyz",
			wantLimit:   0,
		},
		{
			name:        "non-multipart content type gets zero",
			method:      http.MethodPost,
			host:        "partner.localhost:6969",
			path:        "/products/new",
			contentType: "application/x-www-form-urlencoded",
			wantLimit:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Host = tc.host
			req.Header.Set("Content-Type", tc.contentType)
			got := ProductMediaRequestLimit(req)
			if got != tc.wantLimit {
				t.Fatalf("ProductMediaRequestLimit() = %d; want %d", got, tc.wantLimit)
			}
		})
	}
}

func TestIsPositiveDecimal(t *testing.T) {
	if !isPositiveDecimal("123") {
		t.Fatal("123 should be positive decimal")
	}
	if isPositiveDecimal("0") {
		t.Fatal("0 is not positive decimal")
	}
	if isPositiveDecimal("000") {
		t.Fatal("000 is not positive decimal")
	}
	if isPositiveDecimal("-5") {
		t.Fatal("-5 is not positive decimal")
	}
	if isPositiveDecimal("12a3") {
		t.Fatal("12a3 is not positive decimal")
	}
	if isPositiveDecimal("") {
		t.Fatal("empty string is not positive decimal")
	}
}

func createMultipartRequest(t *testing.T, files map[string]struct {
	filename    string
	contentType string
	data        []byte
}) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for fieldName, file := range files {
		part, err := writer.CreateFormFile(fieldName, file.filename)
		if err != nil {
			t.Fatalf("CreateFormFile error: %v", err)
		}
		if _, err := part.Write(file.data); err != nil {
			t.Fatalf("Write part error: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/products/new", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func generateTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			t.Fatalf("jpeg.Encode error: %v", err)
		}
	} else if format == "png" {
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("png.Encode error: %v", err)
		}
	}
	return buf.Bytes()
}

func TestParseProductImagePart(t *testing.T) {
	t.Run("missing image returns nil, nil", func(t *testing.T) {
		req := createMultipartRequest(t, nil)
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		img, err := parseProductImagePart(req)
		if err != nil || img != nil {
			t.Fatalf("expected nil, nil; got %v, %v", img, err)
		}
	})

	t.Run("valid PNG decoded and normalized", func(t *testing.T) {
		pngBytes := generateTestImage(t, "png", 100, 80)
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"image": {filename: "item.png", contentType: "image/png", data: pngBytes},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		upload, err := parseProductImagePart(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upload == nil || upload.ContentType != "image/png" || len(upload.Data) == 0 {
			t.Fatalf("invalid upload result: %+v", upload)
		}
	})

	t.Run("valid JPEG decoded and normalized", func(t *testing.T) {
		jpegBytes := generateTestImage(t, "jpeg", 120, 90)
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"image": {filename: "item.jpg", contentType: "image/jpeg", data: jpegBytes},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		upload, err := parseProductImagePart(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upload == nil || upload.ContentType != "image/jpeg" || len(upload.Data) == 0 {
			t.Fatalf("invalid upload result: %+v", upload)
		}
	})

	t.Run("image exceeding dimension limits rejected", func(t *testing.T) {
		tooWide := generateTestImage(t, "png", 2049, 10)
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"image": {filename: "wide.png", contentType: "image/png", data: tooWide},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		_, err := parseProductImagePart(req)
		if err != partnersvc.ErrInvalidProductImage {
			t.Fatalf("expected ErrInvalidProductImage, got %v", err)
		}
	})

	t.Run("corrupt or non-image data rejected", func(t *testing.T) {
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"image": {filename: "bad.png", contentType: "image/png", data: []byte("not a real image")},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		_, err := parseProductImagePart(req)
		if err != partnersvc.ErrInvalidProductImage {
			t.Fatalf("expected ErrInvalidProductImage, got %v", err)
		}
	})
}

func TestParseProductVideoPart(t *testing.T) {
	t.Run("missing video returns nil, nil", func(t *testing.T) {
		req := createMultipartRequest(t, nil)
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		video, err := parseProductVideoPart(req)
		if err != nil || video != nil {
			t.Fatalf("expected nil, nil; got %v, %v", video, err)
		}
	})

	t.Run("valid MP4 accepted", func(t *testing.T) {
		// Valid MP4 starts with 4 bytes length + 'ftyp' + 4 bytes brand
		mp4Header := []byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
		mp4Data := append(mp4Header, bytes.Repeat([]byte{0x00}, 100)...)

		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"video": {filename: "clip.mp4", contentType: "video/mp4", data: mp4Data},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		upload, err := parseProductVideoPart(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upload == nil || upload.ContentType != "video/mp4" || len(upload.Data) != len(mp4Data) {
			t.Fatalf("invalid video upload: %+v", upload)
		}
	})

	t.Run("valid WebM accepted", func(t *testing.T) {
		// WebM magic bytes: 0x1A 0x45 0xDF 0xA3
		webmData := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x01, 0x00, 0x00}
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"video": {filename: "clip.webm", contentType: "video/webm", data: webmData},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		upload, err := parseProductVideoPart(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if upload == nil || upload.ContentType != "video/webm" || len(upload.Data) != len(webmData) {
			t.Fatalf("invalid video upload: %+v", upload)
		}
	})

	t.Run("mismatched video extension and content rejected", func(t *testing.T) {
		req := createMultipartRequest(t, map[string]struct {
			filename    string
			contentType string
			data        []byte
		}{
			"video": {filename: "clip.mp4", contentType: "video/mp4", data: []byte("definitely not mp4")},
		})
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}
		_, err := parseProductVideoPart(req)
		if err != partnersvc.ErrInvalidProductVideo {
			t.Fatalf("expected ErrInvalidProductVideo, got %v", err)
		}
	})
}

func TestExifOrientation(t *testing.T) {
	t.Run("non-jpeg returns orientation 1", func(t *testing.T) {
		if orient := readJPEGOrientation([]byte("not jpeg")); orient != 1 {
			t.Fatalf("expected 1, got %d", orient)
		}
	})

	t.Run("applyOrientation preserves bounds or rotates", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 10, 20))
		for orient := 1; orient <= 8; orient++ {
			res := applyOrientation(src, orient)
			b := res.Bounds()
			switch orient {
			case 1, 2, 3, 4:
				if b.Dx() != 10 || b.Dy() != 20 {
					t.Fatalf("orient %d: expected 10x20, got %dx%d", orient, b.Dx(), b.Dy())
				}
			case 5, 6, 7, 8:
				if b.Dx() != 20 || b.Dy() != 10 {
					t.Fatalf("orient %d: expected 20x10, got %dx%d", orient, b.Dx(), b.Dy())
				}
			}
		}
	})
}
