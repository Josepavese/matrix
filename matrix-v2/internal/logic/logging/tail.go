package logging

import (
	"bufio"
	"errors"
	"io"
	"time"

	"github.com/jose/matrix-v2/internal/middleware"
)

func TailFile(fs middleware.FS, path string, lines int) ([]string, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if lines < 1 {
		lines = 1
	}

	ring := make([]string, 0, lines)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(ring) == lines {
			copy(ring, ring[1:])
			ring[len(ring)-1] = scanner.Text()
			continue
		}
		ring = append(ring, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ring, nil
}

func FollowFile(fs middleware.FS, path string, fromOffset int64, printLine func(string)) error {
	file, err := fs.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Seek(fromOffset, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err == nil {
			printLine(trimTrailingNewline(line))
			continue
		}
		if !errors.Is(err, io.EOF) {
			return err
		}

		offset, seekErr := file.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return seekErr
		}

		info, statErr := fs.Stat(path)
		if statErr == nil && info.Size() < offset {
			_ = file.Close()
			file, err = fs.Open(path)
			if err != nil {
				return err
			}
			reader = bufio.NewReader(file)
			continue
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func trimTrailingNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
