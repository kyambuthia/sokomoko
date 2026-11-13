package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

func saveProductVideoTx(ctx context.Context, q querier, productID int, video *ProductVideo) error {
	if err := validateProductVideo(video); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO product_video_blobs (product_id, content_type, data)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (product_id) DO UPDATE SET
		     content_type = EXCLUDED.content_type,
		     data = EXCLUDED.data,
		     created_at = now()`,
		productID, video.ContentType, video.Data,
	)
	return err
}

// deleteUploadedProductImagesTx removes images that have stored bytes.
// Rows with only a URL, such as bundled gallery images, are left in place.
// Deleting the image row cascades to product_image_blobs.
func deleteUploadedProductImagesTx(ctx context.Context, q querier, productID int) error {
	_, err := q.ExecContext(ctx,
		`DELETE FROM product_images AS image
		 USING product_image_blobs AS blob
		 WHERE image.id = blob.product_image_id AND image.product_id = $1`,
		productID,
	)
	return err
}

func insertProductImageTx(ctx context.Context, q querier, productID int64, image ProductImage, position int) error {
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
	if _, err := q.ExecContext(ctx,
		"INSERT INTO product_image_blobs (product_image_id, content_type, data) VALUES ($1, $2, $3)",
		imageID, image.ContentType, image.Data,
	); err != nil {
		return err
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
	err := s.DB.QueryRowContext(ctx,
		`SELECT i.id, i.product_id, b.content_type, b.data, i.created_at
		 FROM product_images i
		 JOIN product_image_blobs b ON b.product_image_id = i.id
		 JOIN products p ON p.id = i.product_id
		 WHERE i.id = $1 AND p.deleted_at IS NULL AND `+activePartnerSQL,
		id,
	).Scan(&image.ID, &image.ProductID, &image.ContentType, &image.Data, &image.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	return image, nil
}

// GetPublicProductVideoByProductID returns a video only while its product and
// partner are public.
func (s *Store) GetPublicProductVideoByProductID(ctx context.Context, productID int) (*ProductVideo, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	video := &ProductVideo{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT v.product_id, v.content_type, v.data, v.created_at
		 FROM product_video_blobs v
		 JOIN products p ON p.id = v.product_id
		 WHERE v.product_id = $1 AND p.deleted_at IS NULL AND `+activePartnerSQL,
		productID,
	).Scan(&video.ProductID, &video.ContentType, &video.Data, &video.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
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
