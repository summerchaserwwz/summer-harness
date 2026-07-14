package continuity

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeHandoffNotFound          Code = "HANDOFF_NOT_FOUND"
	CodeHandoffTooLarge          Code = "HANDOFF_TOO_LARGE"
	CodeHandoffInvalid           Code = "HANDOFF_INVALID"
	CodeHandoffUnsupportedSchema Code = "HANDOFF_UNSUPPORTED_SCHEMA"
	CodeHandoffDrift             Code = "HANDOFF_DRIFT"
	CodeUnsafeReference          Code = "UNSAFE_REFERENCE"
	CodeGSDPointerStale          Code = "GSD_POINTER_STALE"
	CodeCapsuleTooLarge          Code = "CAPSULE_TOO_LARGE"
	CodeProjectionStale          Code = "PROJECTION_STALE"
	CodeProjectionConflict       Code = "PROJECTION_CONFLICT"
	CodeCapabilityUnavailable    Code = "CAPABILITY_UNAVAILABLE"
	CodeMigrationRequired        Code = "MIGRATION_REQUIRED"
	CodeLifecycleConflict        Code = "LIFECYCLE_CONFLICT"
)

type Error struct {
	Code Code
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	message := string(e.Code)
	if e.Op != "" {
		message = e.Op + ": " + message
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *Error) Unwrap() error { return e.Err }

func ErrorCode(err error) Code {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

func wrap(code Code, op, path string, err error) error {
	if err == nil {
		err = fmt.Errorf("continuity operation failed")
	}
	return &Error{Code: code, Op: op, Path: path, Err: err}
}
