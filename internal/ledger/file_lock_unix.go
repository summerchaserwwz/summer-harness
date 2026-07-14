//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func acquireProcessFileLock(ctx context.Context, path string, owner *writeLockOwner) (func() error, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("ledger write lock is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect ledger write lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open ledger write lock: %w", err)
	}
	closeWithError := func(err error) (func() error, error) {
		if closeErr := file.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}

	for {
		if err := ctx.Err(); err != nil {
			return closeWithError(err)
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return closeWithError(fmt.Errorf("acquire ledger write lock: %w", err))
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return closeWithError(ctx.Err())
		case <-timer.C:
		}
	}

	if owner != nil {
		payload, err := json.Marshal(owner)
		if err != nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return closeWithError(fmt.Errorf("encode ledger lock owner: %w", err))
		}
		if err := file.Truncate(0); err != nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return closeWithError(fmt.Errorf("truncate ledger lock owner: %w", err))
		}
		if _, err := file.Seek(0, 0); err != nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return closeWithError(fmt.Errorf("seek ledger lock owner: %w", err))
		}
		if _, err := file.Write(append(payload, '\n')); err != nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return closeWithError(fmt.Errorf("write ledger lock owner: %w", err))
		}
		if err := file.Sync(); err != nil {
			_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			return closeWithError(fmt.Errorf("sync ledger lock owner: %w", err))
		}
	}

	return func() error {
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("release ledger write lock: %w", unlockErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close ledger write lock: %w", closeErr)
		}
		return errors.Join(unlockErr, closeErr)
	}, nil
}
