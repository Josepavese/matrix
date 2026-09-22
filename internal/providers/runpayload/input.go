package runpayload

import (
	"encoding/json"
	"fmt"
)

// Input accepts both the compact string form and the structured run body form.
type Input string

func (i *Input) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*i = Input(text)
		return nil
	}
	// A pointer distinguishes "text is present but empty", which is a legitimate
	// empty turn, from "text is absent", which is a malformed request. Without
	// that distinction a misspelled key produced a successful run with no output.
	var structured struct {
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(data, &structured); err != nil {
		return fmt.Errorf("input must be a string or object with text: %w", err)
	}
	if structured.Text == nil {
		return fmt.Errorf("input must be a string or object with text")
	}
	*i = Input(*structured.Text)
	return nil
}

func (i Input) String() string {
	return string(i)
}
