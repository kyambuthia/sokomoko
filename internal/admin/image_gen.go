package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
)

const (
	DeepInfraAPIEndpoint = "https://api.deepinfra.com/v1/inference/Bria/Bria-3.2"
)

type ImageGenRequest struct {
	Prompt            string `json:"prompt"`
	NegativePrompt    string `json:"negative_prompt,omitempty"`
	Height            int    `json:"height"`
	Width             int    `json:"width"`
	NumResults        int    `json:"num_results"`
	NumInferenceSteps int    `json:"num_inference_steps,omitempty"`
}

type ImageGenResponse struct {
	Images []string `json:"images"` // URLs or base64 data
	Status string   `json:"status"`
}

// GenerateProductImages generates three professional images for a product using DeepInfra
func GenerateProductImages(product db.Product) ([]db.ProductImage, error) {
	apiKey := os.Getenv("DEEPINFRA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DEEPINFRA_API_KEY environment variable is not set")
	}

	angles := []struct {
		suffix string
		prompt string
	}{
		{"front", "front-facing professional studio product shot, white background, high resolution"},
		{"side", "side-angle professional studio product shot, white background, high resolution"},
		{"macro", "close-up macro detail shot, professional lighting, white background, high resolution"},
	}

	var productImages []db.ProductImage
	basePath := filepath.Join("internal", "ui", "static", "images", "products", product.Slug)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	for i, angle := range angles {
		fullPrompt := fmt.Sprintf("%s, %s, professional e-commerce product photography, isolated on white background, sharp focus, 8k", product.Name, angle.prompt)

		reqBody := ImageGenRequest{
			Prompt:         fullPrompt,
			NegativePrompt: "background, shadows, human, hands, blurry, low quality, watermarks, text",
			Height:         200,
			Width:          200,
			NumResults:     1,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", DeepInfraAPIEndpoint, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error calling DeepInfra for %s (%s): %v", product.Name, angle.suffix, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("DeepInfra API error: %s - %s", resp.Status, string(body))
			continue
		}

		var result ImageGenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		if len(result.Images) > 0 {
			imageURL := result.Images[0]
			fileName := fmt.Sprintf("%s_%s.jpg", product.Slug, angle.suffix)
			filePath := filepath.Join(basePath, fileName)

			// Download and save image
			if err := downloadAndSaveImage(imageURL, filePath); err != nil {
				log.Printf("Error saving image %s: %v", fileName, err)
				continue
			}

			// Add to product images slice
			productImages = append(productImages, db.ProductImage{
				ProductID:    product.ID,
				URL:          fmt.Sprintf("/static/images/products/%s/%s", product.Slug, fileName),
				AltText:      fmt.Sprintf("%s - %s", product.Name, angle.suffix),
				DisplayOrder: i,
			})
		}
	}

	return productImages, nil
}

func downloadAndSaveImage(url string, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
