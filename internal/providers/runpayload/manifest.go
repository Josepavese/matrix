package runpayload

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PartEvidence intentionally excludes content and caller-controlled metadata.
// Context refs plus the payload digest bind source SHA/page/revision to the run.
type PartEvidence struct {
	Type   string `json:"type"`
	MIME   string `json:"mime_type,omitempty"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func (i Input) Manifest() []PartEvidence {
	parts := make([]PartEvidence, 0, len(i.Content))
	for _, block := range i.Content {
		data := []byte(block.Text)
		if block.Type == ContentImage {
			data, _ = base64.StdEncoding.DecodeString(block.Data)
		}
		parts = append(parts, PartEvidence{Type: block.Type, MIME: block.MimeType, Bytes: len(data), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return parts
}
