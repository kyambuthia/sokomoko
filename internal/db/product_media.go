package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/blob"
)

func hasUploadedProductImage(images []ProductImage) bool {
	for _, image := range images {
		if len(image.Data) > 0 {
			return true
		}
	}
	return false
}

func validateProductVideo(video *ProductVideo) error {
	if video == nil {
		return nil
	}
	if len(video.Data) == 0 || len(video.Data) > MaxProductVideoBytes ||
		(video.ContentType != "video/mp4" && video.ContentType != "video/webm") {
		return ErrInvalidProductVideo
	}
	return nil
}

func (s *Store) saveProductVideoTx(ctx context.Context, q querier, productID int, video *ProductVideo) error {
	if err := validateProductVideo(video); err != nil {
		return err
	}
	blobKey := fmt.Sprintf("products/%d/video", productID)
	if s.blobs != nil {
		if err := s.blobs.Put(ctx, blobKey, video.ContentType, video.Data); err != nil {
			return err
		}
		_, err := q.ExecContext(ctx,
			`INSERT INTO product_video_blobs (product_id, content_type, blob_key, data)
			 VALUES ($1, $2, $3, NULL)
			 ON CONFLICT (product_id) DO UPDATE SET
			     content_type = EXCLUDED.content_type,
			     blob_key = EXCLUDED.blob_key,
			     data = NULL,
			     created_at = now()`,
			productID, video.ContentType, blobKey,
		)
		return err
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO product_video_blobs (product_id, content_type, data)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (product_id) DO UPDATE SET
		     content_type = EXCLUDED.content_type,
		     blob_key = NULL,
		     data = EXCLUDED.data,
		     created_at = now()`,
		productID, video.ContentType, video.Data,
	)
	return err
}

func (s *Store) deleteProductVideoTx(ctx context.Context, q querier, productID int) error {
	var blobKey string
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(blob_key, '') FROM product_video_blobs WHERE product_id = $1`,
		productID,
	).Scan(&blobKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := q.ExecContext(ctx,
		`DELETE FROM product_video_blobs WHERE product_id = $1`,
		productID,
	); err != nil {
		return err
	}
	if blobKey != "" && s.blobs != nil {
		_ = s.blobs.Delete(ctx, blobKey)
	}
	return nil
}

// deleteUploadedProductImagesTx removes images that have stored blobs.
// Rows with only a URL, such as bundled gallery images, are left in place.
// Deleting the image row cascades to product_image_blobs.
func (s *Store) deleteUploadedProductImagesTx(ctx context.Context, q querier, productID int) error {
	var keys []string
	rows, err := q.QueryContext(ctx,
		`SELECT COALESCE(blob.blob_key, '')
		 FROM product_images AS image
		 JOIN product_image_blobs AS blob ON blob.product_image_id = image.id
		 WHERE image.product_id = $1`,
		productID,
	)
	if err != nil {
		return err
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return err
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	if _, err := q.ExecContext(ctx,
		`DELETE FROM product_images AS image
		 USING product_image_blobs AS blob
		 WHERE image.id = blob.product_image_id AND image.product_id = $1`,
		productID,
	); err != nil {
		return err
	}
	if s.blobs != nil {
		for _, key := range keys {
			_ = s.blobs.Delete(ctx, key)
		}
	}
	return nil
}

func (s *Store) insertProductImageTx(ctx context.Context, q querier, productID int64, image ProductImage, position int) error {
	if len(image.Data) == 0 {
		_, err := q.ExecContext(ctx,
			"INSERT INTO product_images (product_id, url, alt_text, position) VALUES ($1, $2, $3, $4)",
			productID, strings.TrimSpace(image.URL), strings.TrimSpace(image.AltText), position,
		)
		return err
	}

	if len(image.Data) > MaxProductImageBytes || (image.ContentType != "image/jpeg" && image.ContentType != "image/png") {
		return ErrInvalidProductImage
	}
	var imageID int64
	if err := q.QueryRowContext(ctx,
		`INSERT INTO product_images (product_id, url, alt_text, position)
		 VALUES ($1, '/media/product-images/pending', $2, $3) RETURNING id`,
		productID, strings.TrimSpace(image.AltText), position,
	).Scan(&imageID); err != nil {
		return err
	}
	if s.blobs != nil {
		blobKey := fmt.Sprintf("products/%d/images/%d", productID, imageID)
		if err := s.blobs.Put(ctx, blobKey, image.ContentType, image.Data); err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx,
			"INSERT INTO product_image_blobs (product_image_id, content_type, blob_key, data) VALUES ($1, $2, $3, NULL)",
			imageID, image.ContentType, blobKey,
		); err != nil {
			_ = s.blobs.Delete(ctx, blobKey)
			return err
		}
	} else {
		if _, err := q.ExecContext(ctx,
			"INSERT INTO product_image_blobs (product_image_id, content_type, data) VALUES ($1, $2, $3)",
			imageID, image.ContentType, image.Data,
		); err != nil {
			return err
		}
	}
	_, err := q.ExecContext(ctx,
		"UPDATE product_images SET url = $1 WHERE id = $2",
		fmt.Sprintf("/media/product-images/%d", imageID), imageID,
	)
	return err
}

// GetPublicProductImageByID returns image bytes only for an active public product.
func (s *Store) GetPublicProductImageByID(ctx context.Context, id int) (*ProductImage, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	image := &ProductImage{}
	var blobKey string
	err := s.DB.QueryRowContext(ctx,
		`SELECT i.id, i.product_id, b.content_type, COALESCE(b.blob_key, ''), b.data, i.created_at
		 FROM product_images i
		 JOIN product_image_blobs b ON b.product_image_id = i.id
		 JOIN products p ON p.id = i.product_id
		 WHERE i.id = $1 AND p.deleted_at IS NULL AND `+activePartnerSQL,
		id,
	).Scan(&image.ID, &image.ProductID, &image.ContentType, &blobKey, &image.Data, &image.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if blobKey != "" {
		if s.blobs == nil {
			return nil, ErrProductNotFound
		}
		data, contentType, err := s.blobs.Get(ctx, blobKey)
		if errors.Is(err, blob.ErrNotFound) {
			return nil, ErrProductNotFound
		}
		if err != nil {
			return nil, err
		}
		image.Data = data
		if contentType != "" && contentType != "application/octet-stream" {
			image.ContentType = contentType
		}
	}
	if len(image.Data) == 0 {
		return nil, ErrProductNotFound
	}
	return image, nil
}

// GetPublicProductVideoByProductID returns a video only while its product and
// partner are public.
func (s *Store) GetPublicProductVideoByProductID(ctx context.Context, productID int) (*ProductVideo, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	video := &ProductVideo{}
	var blobKey string
	err := s.DB.QueryRowContext(ctx,
		`SELECT v.product_id, v.content_type, COALESCE(v.blob_key, ''), v.data, v.created_at
		 FROM product_video_blobs v
		 JOIN products p ON p.id = v.product_id
		 WHERE v.product_id = $1 AND p.deleted_at IS NULL AND `+activePartnerSQL,
		productID,
	).Scan(&video.ProductID, &video.ContentType, &blobKey, &video.Data, &video.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	if blobKey != "" {
		if s.blobs == nil {
			return nil, ErrProductNotFound
		}
		data, contentType, err := s.blobs.Get(ctx, blobKey)
		if errors.Is(err, blob.ErrNotFound) {
			return nil, ErrProductNotFound
		}
		if err != nil {
			return nil, err
		}
		video.Data = data
		if contentType != "" && contentType != "application/octet-stream" {
			video.ContentType = contentType
		}
	}
	if len(video.Data) == 0 {
		return nil, ErrProductNotFound
	}
	return video, nil
}

func getProductImagesByProductIDs(ctx context.Context, q querier, productIDs []int) (map[int][]ProductImage, error) {
	imagesByProduct := map[int][]ProductImage{}
	if len(productIDs) == 0 {
		return imagesByProduct, nil
	}

	rows, err := q.QueryContext(ctx,
		`SELECT id, product_id, url, alt_text, position, created_at
		 FROM product_images
		 WHERE product_id = ANY($1)
		 ORDER BY product_id, position, id`,
		int64Slice(productIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		img := ProductImage{}
		if err := rows.Scan(&img.ID, &img.ProductID, &img.URL, &img.AltText, &img.DisplayOrder, &img.CreatedAt); err != nil {
			return nil, err
		}
		imagesByProduct[img.ProductID] = append(imagesByProduct[img.ProductID], img)
	}
	return imagesByProduct, rows.Err()
}
