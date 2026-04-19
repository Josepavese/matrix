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
	var structured struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &structured); err != nil {
		return fmt.Errorf("input must be a string or object with text: %w", err)
	}
	*i = Input(structured.Text)
	return nil
}

func (i Input) String() string {
	return string(i)
}
