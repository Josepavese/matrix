package runpayload

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/jpeg" // Register only the inline formats this boundary accepts.
	_ "image/png"  // Register PNG decoding for byte/MIME validation.
	"strings"

	"github.com/Josepavese/matrix/internal/middleware"
)

// These bounded inline-input policies belong to the run payload owner. No URLs,
// filesystem references, image fetching or persistent blob storage are implicit.
const (
	MaxBlocks          = 16
	MaxImageBytes      = 512 << 10
	MaxTotalImageBytes = 512 << 10
	MaxImageDimension  = 8192
	MaxImagePixels     = 4 << 20
	ContentText        = "text"
	ContentImage       = "image"
)

var imageMIMEs = map[string]string{"png": "image/png", "jpeg": "image/jpeg"}

func (i Input) Validate() error {
	if len(i.Content) > MaxBlocks {
		return invalidInput("input content exceeds %d blocks", MaxBlocks)
	}
	total := 0
	for _, block := range i.Content {
		if err := validateBlock(block); err != nil {
			return err
		}
		if block.Type == ContentImage {
			decoded, err := decodeImage(block)
			if err != nil {
				return err
			}
			total += len(decoded)
			if total > MaxTotalImageBytes {
				return invalidInput("input images exceed %d decoded bytes", MaxTotalImageBytes)
			}
		}
	}
	return nil
}

func validateBlock(block middleware.Content) error {
	if unsupportedFields(block) {
		return invalidInput("input content contains unsupported fields; only inline text and images are accepted")
	}
	switch block.Type {
	case ContentText:
		if strings.TrimSpace(block.Text) == "" || block.Data != "" || block.MimeType != "" {
			return invalidInput("input text block must contain only nonempty text and optional metadata")
		}
	case ContentImage:
		if block.Text != "" || block.Data == "" {
			return invalidInput("input image must contain inline data, MIME and optional metadata")
		}
	default:
		return invalidInput("input content type unsupported")
	}
	return nil
}

func decodeImage(block middleware.Content) ([]byte, error) {
	if len(block.Data) > base64.StdEncoding.EncodedLen(MaxImageBytes) {
		return nil, invalidInput("input image exceeds %d decoded bytes", MaxImageBytes)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(block.Data)
	if err != nil || len(decoded) > MaxImageBytes || base64.StdEncoding.EncodeToString(decoded) != block.Data {
		return nil, invalidInput("input image requires canonical base64")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || imageMIMEs[format] == "" || imageMIMEs[format] != block.MimeType {
		return nil, invalidInput("input image bytes do not match supported PNG/JPEG MIME")
	}
	if !validDimensions(config) {
		return nil, invalidInput("input image dimensions exceed limits")
	}
	if _, _, err := image.Decode(bytes.NewReader(decoded)); err != nil {
		return nil, invalidInput("input image is incomplete or invalid")
	}
	return decoded, nil
}

func unsupportedFields(block middleware.Content) bool {
	return block.URI != "" || block.Resource != nil || block.Name != "" || block.Title != "" || block.Description != "" || block.Size != nil || block.Annotations != nil
}

func validDimensions(config image.Config) bool {
	return config.Width > 0 && config.Height > 0 && config.Width <= MaxImageDimension && config.Height <= MaxImageDimension && int64(config.Width)*int64(config.Height) <= MaxImagePixels
}
