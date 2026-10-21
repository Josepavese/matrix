package runpayload

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/strictjson"
	"github.com/Josepavese/matrix/internal/middleware"
)

// Input is the canonical run payload. Compact text and structured content are
// current encodings of the same contract, not distinct execution paths.
type Input struct {
	Text    string               `json:"text,omitempty"`
	Content []middleware.Content `json:"content,omitempty"`
}

func (i *Input) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return invalidInput("input cannot be null")
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*i = Input{Text: text}
		return nil
	}
	type payload Input
	var parsed payload
	if err := strictjson.Decode(bytes.NewReader(data), &parsed); err != nil {
		return err
	}
	candidate := Input(parsed)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*i = candidate
	return nil
}

func (i Input) MarshalJSON() ([]byte, error) {
	if len(i.Content) == 0 {
		return json.Marshal(i.Text)
	}
	type payload Input
	return json.Marshal(payload(i))
}

func (i Input) String() string { return i.Text }

func (i Input) Empty() bool { return strings.TrimSpace(i.Text) == "" && len(i.Content) == 0 }

// Digest covers the whole canonical payload, including content order and metadata.
func (i Input) Digest() string {
	encoded, _ := json.Marshal(i)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}
