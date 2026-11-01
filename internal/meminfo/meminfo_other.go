//go:build !darwin && !linux && !windows

package meminfo

import "errors"

// ErrUnsupportedOS is returned on platforms the product does not ship to. The
// stub exists so a cross-compile or a CI job on another GOOS links rather than
// failing on a missing symbol; callers already fall back to the engine default
// when Total reports an error.
var ErrUnsupportedOS = errors.New("memory size detection is not supported on this OS")

func totalRAM() (uint64, error) { return 0, ErrUnsupportedOS }
