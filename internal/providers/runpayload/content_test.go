package runpayload

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

func picture(t *testing.T, c color.RGBA) middleware.Content {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	im.Set(3, 4, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return middleware.Content{Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(buf.Bytes())}
}

func TestInputPreservesImagesAndDigest(t *testing.T) {
	original := Input{Text: "inspect", Content: []middleware.Content{picture(t, color.RGBA{R: 255, A: 255})}}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Input
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Content[0].Data != original.Content[0].Data || decoded.Digest() != original.Digest() {
		t.Fatal("content lost")
	}
	changed := Input{Text: "inspect", Content: []middleware.Content{picture(t, color.RGBA{B: 255, A: 255})}}
	if changed.Digest() == original.Digest() {
		t.Fatal("image absent from digest")
	}
	changed = original
	changed.Content = append([]middleware.Content(nil), original.Content...)
	changed.Content[0].Meta = map[string]any{"page": 2, "revision": "r2"}
	if changed.Digest() == original.Digest() {
		t.Fatal("source binding absent from digest")
	}
	manifest, _ := json.Marshal(original.Manifest())
	if bytes.Contains(manifest, []byte(original.Content[0].Data)) || !bytes.Contains(manifest, []byte(`"mime_type":"image/png"`)) {
		t.Fatal("unsafe manifest")
	}
}

func TestInputRejectsInvalidContentAndAmbiguousFields(t *testing.T) {
	for _, raw := range []string{`null`, `{"text":"x","unknown":1}`, `{"text":"x","text":"y"}`, `{"Text":"x"}`, `{"content":[{"type":"image","data":"x","mimeType":"image/png","secret":1}]}`} {
		var in Input
		if err := json.Unmarshal([]byte(raw), &in); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	valid := picture(t, color.RGBA{R: 255, A: 255})
	cases := []middleware.Content{
		{Type: "image", MimeType: "image/png", URI: "https://example.invalid/image.png"},
		{Type: "audio", Data: "eA=="}, {Type: "text", Text: " "},
		{Type: "image", MimeType: "image/png", Data: "eA=="},
		{Type: "image", MimeType: "image/png", Data: strings.Repeat("A", base64.StdEncoding.EncodedLen(MaxImageBytes)+1)},
	}
	mismatch := valid
	mismatch.MimeType = "image/jpeg"
	cases = append(cases, mismatch)
	noncanonical := valid
	noncanonical.Data += "\n"
	cases = append(cases, noncanonical)
	mixed := valid
	mixed.Text = "ignored"
	cases = append(cases, mixed)
	for _, block := range cases {
		if err := (Input{Content: []middleware.Content{block}}).Validate(); err == nil {
			t.Fatal("accepted invalid content")
		}
	}
	if err := (Input{Content: make([]middleware.Content, MaxBlocks+1)}).Validate(); err == nil {
		t.Fatal("block limit bypass")
	}
}

func TestInputRejectsImageDimensionBomb(t *testing.T) {
	for _, rect := range []image.Rectangle{image.Rect(0, 0, 1, MaxImageDimension+1), image.Rect(0, 0, 2049, 2049)} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewGray(rect)); err != nil {
			t.Fatal(err)
		}
		in := Input{Content: []middleware.Content{{Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(buf.Bytes())}}}
		if err := in.Validate(); err == nil {
			t.Fatal("image bomb accepted")
		}
	}
}

func FuzzInput(f *testing.F) {
	for _, raw := range []string{`"text"`, `{"text":"x"}`, `{"content":[]}`, `{"content":[{"type":"image","data":"eA=="}]}`} {
		f.Add(raw)
	}
	f.Fuzz(func(_ *testing.T, raw string) {
		var in Input
		if json.Unmarshal([]byte(raw), &in) == nil {
			_, _ = json.Marshal(in)
		}
	})
}

func TestInputRejectsAggregateImageBytesAndAcceptsJPEG(t *testing.T) {
	im := image.NewGray(image.Rect(0, 0, 400, 800))
	rng := rand.New(rand.NewSource(7))
	_, _ = rng.Read(im.Pix)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, im); err != nil {
		t.Fatal(err)
	}
	block := middleware.Content{Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(pngData.Bytes())}
	if err := (Input{Content: []middleware.Content{block}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Input{Content: []middleware.Content{block, block}}).Validate(); err == nil {
		t.Fatal("aggregate byte limit bypassed")
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	input := Input{Content: []middleware.Content{{Type: "image", MimeType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(jpegData.Bytes())}}}
	if err := input.Validate(); err != nil || input.Empty() {
		t.Fatal("image-only JPEG rejected", err)
	}
}
