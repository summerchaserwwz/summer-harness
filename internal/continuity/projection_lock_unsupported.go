//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package continuity

import (
	"context"
	"errors"
)

func acquireProjectionLock(context.Context, string) (func() error, error) {
	return nil, errors.New("cross-process projection locking is not supported on this platform yet")
}

func fsyncProjectionDirectory(string) error {
	return errors.New("directory fsync is not supported on this platform yet")
}
