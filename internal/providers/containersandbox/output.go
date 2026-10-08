package containersandbox

// Ignore excess bytes while recording overflow: bounded even for a hostile
// helper, with no partial JSON accepted and no retained stderr body.
type boundedOutput struct {
	data     []byte
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	remaining := 65536 - len(b.data)
	if len(p) > remaining {
		b.overflow = true
		b.data = append(b.data, p[:remaining]...)
	} else {
		b.data = append(b.data, p...)
	}
	return len(p), nil
}
