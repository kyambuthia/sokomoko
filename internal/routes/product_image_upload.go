package routes

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

const (
	maxProductImageInputBytes   = 900 * 1024
	maxProductImageRequestBytes = app.MaxRequestBodyBytes
	maxProductImageDimension    = 2048
	maxProductImagePixels       = 4_000_000
	productImageUploadSlots     = 64
	productImageDecodeSlots     = 2
)

const invalidProductImageMessage = "Use a JPEG or PNG under 900 KB and no larger than 2048 × 2048 pixels; optimized images must be under 256 KB."
const productImageBusyMessage = "Image uploads are busy. Try again shortly."

var (
	errProductImageBusy         = errors.New("product image upload capacity reached")
	productImageUploadSemaphore = make(chan struct{}, productImageUploadSlots)
	productImageDecodeSemaphore = make(chan struct{}, productImageDecodeSlots)
)

func parseProductImageUpload(w http.ResponseWriter, r *http.Request) (*partnersvc.ProductImageUpload, error) {
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxProductImageRequestBytes)
	}
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
	// Bound concurrent multipart bodies and temporary files per process. The
	// smaller decode semaphore separately limits image memory and CPU.
	select {
	case productImageUploadSemaphore <- struct{}{}:
		defer func() { <-productImageUploadSemaphore }()
	default:
		w.Header().Set("Retry-After", "1")
		return nil, errProductImageBusy
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

	select {
	case productImageDecodeSemaphore <- struct{}{}:
		defer func() { <-productImageDecodeSemaphore }()
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}

	raw, err := io.ReadAll(io.LimitReader(file, maxProductImageInputBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxProductImageInputBytes {
		return nil, partnersvc.ErrInvalidProductImage
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
		orientation := readJPEGOrientation(raw)
		decoded = applyOrientation(decoded, orientation)
		err = jpeg.Encode(&normalized, decoded, &jpeg.Options{Quality: 80})
	} else {
		err = png.Encode(&normalized, decoded)
	}
	if err != nil || normalized.Len() == 0 || normalized.Len() > partnersvc.MaxProductImageBytes {
		return nil, partnersvc.ErrInvalidProductImage
	}
	return &partnersvc.ProductImageUpload{ContentType: storedType, Data: normalized.Bytes()}, nil
}

func readJPEGOrientation(raw []byte) int {
	if len(raw) < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return 1
	}

	idx := 2
	for idx < len(raw) {
		for idx < len(raw) && raw[idx] != 0xFF {
			idx++
		}
		for idx < len(raw) && raw[idx] == 0xFF {
			idx++
		}
		if idx >= len(raw) {
			break
		}
		marker := raw[idx]
		idx++

		if marker == 0x00 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01 {
			continue
		}
		if marker == 0xD9 || marker == 0xDA {
			break
		}

		if idx+2 > len(raw) {
			break
		}
		length := int(binary.BigEndian.Uint16(raw[idx : idx+2]))
		if length < 2 || idx+length > len(raw) {
			break
		}
		segment := raw[idx+2 : idx+length]
		idx += length

		if marker == 0xE1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			if orient := parseTIFFOrientation(segment[6:]); orient >= 1 && orient <= 8 {
				return orient
			}
		}
	}
	return 1
}

func parseTIFFOrientation(tiffData []byte) int {
	if len(tiffData) < 8 {
		return 1
	}

	var byteOrder binary.ByteOrder
	switch {
	case tiffData[0] == 'I' && tiffData[1] == 'I':
		byteOrder = binary.LittleEndian
	case tiffData[0] == 'M' && tiffData[1] == 'M':
		byteOrder = binary.BigEndian
	default:
		return 1
	}

	if byteOrder.Uint16(tiffData[2:4]) != 42 {
		return 1
	}

	firstIFD := byteOrder.Uint32(tiffData[4:8])
	if firstIFD < 8 || firstIFD > uint32(len(tiffData)-2) {
		return 1
	}

	orient, found, subIFD := readIFDOrientation(tiffData, int(firstIFD), byteOrder)
	if found {
		return orient
	}
	if subIFD >= 8 && subIFD <= len(tiffData)-2 {
		if orientSub, foundSub, _ := readIFDOrientation(tiffData, subIFD, byteOrder); foundSub {
			return orientSub
		}
	}
	return 1
}

func readIFDOrientation(tiffData []byte, ifdOffset int, byteOrder binary.ByteOrder) (orientation int, found bool, subIFD int) {
	if ifdOffset < 8 || ifdOffset > len(tiffData)-6 {
		return 1, false, 0
	}
	numEntries := int(byteOrder.Uint16(tiffData[ifdOffset : ifdOffset+2]))
	offset := ifdOffset + 2
	// Each IFD ends with a four-byte pointer to the next IFD. Validate the
	// complete entry table before trusting any orientation value in it.
	availableEntries := (len(tiffData) - offset - 4) / 12
	if numEntries > availableEntries {
		return 1, false, 0
	}

	for i := 0; i < numEntries; i++ {
		entryOffset := offset + i*12
		if entryOffset < 0 || entryOffset > len(tiffData)-12 {
			break
		}
		tag := byteOrder.Uint16(tiffData[entryOffset : entryOffset+2])
		tagType := byteOrder.Uint16(tiffData[entryOffset+2 : entryOffset+4])
		count := byteOrder.Uint32(tiffData[entryOffset+4 : entryOffset+8])

		if tag == 0x0112 && count == 1 {
			val := 0
			switch tagType {
			case 1: // BYTE
				val = int(tiffData[entryOffset+8])
			case 3: // SHORT
				val = int(byteOrder.Uint16(tiffData[entryOffset+8 : entryOffset+10]))
			case 4: // LONG
				val = int(byteOrder.Uint32(tiffData[entryOffset+8 : entryOffset+12]))
			default:
				continue
			}
			if val >= 1 && val <= 8 {
				return val, true, 0
			}
			return 1, true, 0
		} else if tag == 0x8769 && count == 1 && (tagType == 3 || tagType == 4) {
			if tagType == 3 {
				subIFD = int(byteOrder.Uint16(tiffData[entryOffset+8 : entryOffset+10]))
			} else {
				subIFD = int(byteOrder.Uint32(tiffData[entryOffset+8 : entryOffset+12]))
			}
		}
	}
	return 1, false, subIFD
}

func applyOrientation(img image.Image, orientation int) image.Image {
	if img == nil || orientation <= 1 || orientation > 8 {
		return img
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return img
	}

	var dstW, dstH int
	if orientation >= 5 {
		dstW, dstH = h, w
	} else {
		dstW, dstH = w, h
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	for y := 0; y < h; y++ {
		srcY := bounds.Min.Y + y
		for x := 0; x < w; x++ {
			srcX := bounds.Min.X + x

			var dstX, dstY int
			switch orientation {
			case 2: // Flip horizontal
				dstX, dstY = w-1-x, y
			case 3: // Rotate 180
				dstX, dstY = w-1-x, h-1-y
			case 4: // Flip vertical
				dstX, dstY = x, h-1-y
			case 5: // Transpose (flip horizontal + rotate 270 CW)
				dstX, dstY = y, x
			case 6: // Rotate 90 CW
				dstX, dstY = h-1-y, x
			case 7: // Transverse (flip horizontal + rotate 90 CW)
				dstX, dstY = h-1-y, w-1-x
			case 8: // Rotate 270 CW
				dstX, dstY = y, w-1-x
			}

			dstIdx := (dstY*dstW + dstX) * 4
			r, g, b, a := img.At(srcX, srcY).RGBA()
			dst.Pix[dstIdx] = uint8(r >> 8)
			dst.Pix[dstIdx+1] = uint8(g >> 8)
			dst.Pix[dstIdx+2] = uint8(b >> 8)
			dst.Pix[dstIdx+3] = uint8(a >> 8)
		}
	}

	return dst
}
