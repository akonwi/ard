package diagnostics

import "errors"

// ReportedError marks a failure whose user-facing diagnostics have already
// been rendered. Command boundaries should preserve the failing exit status
// without printing the sentinel error again.
type ReportedError struct {
	Err error
}

func (e *ReportedError) Error() string {
	if e == nil || e.Err == nil {
		return "diagnostics already reported"
	}
	return e.Err.Error()
}

func (e *ReportedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func AlreadyReported(err error) error {
	if err == nil {
		return nil
	}
	return &ReportedError{Err: err}
}

func IsAlreadyReported(err error) bool {
	var reported *ReportedError
	return errors.As(err, &reported)
}
