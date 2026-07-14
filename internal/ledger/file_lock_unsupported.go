//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package ledger

import (
	"context"
	"errors"
)

func acquireProcessFileLock(context.Context, string, *writeLockOwner) (func() error, error) {
	return nil, errors.New("cross-process ledger locking is not supported on this platform yet")
}
