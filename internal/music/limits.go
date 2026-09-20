package music

import (
	"fmt"
	"os"
)

const (
	// MaxFileBytes is Telegram Bot API upload cap for bots (50 MB).
	MaxFileBytes int64 = 50 * 1024 * 1024
)

// ErrTooLarge is returned when a downloaded file exceeds MaxFileBytes.
type ErrTooLarge struct {
	Size int64
}

func (e *ErrTooLarge) Error() string {
	return fmt.Sprintf("file too large for Telegram (%d bytes > %d)", e.Size, MaxFileBytes)
}

// CheckFileSize returns ErrTooLarge if path exceeds MaxFileBytes.
func CheckFileSize(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Size() > MaxFileBytes {
		return &ErrTooLarge{Size: fi.Size()}
	}
	return nil
}

// IsTooLarge reports whether err is (or wraps) ErrTooLarge.
func IsTooLarge(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ErrTooLarge)
	return ok
}
