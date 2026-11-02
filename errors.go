// Package errors provides standard-compatible errors with stack traces.
//
// New, Errorf, and Join capture the calling goroutine's stack. Ordinary
// formatting preserves the error message; %+v prints the error tree and traces.
// Use Formatter when a standard-library wrapper is the outermost error.
package errors

import (
	stderrors "errors"
	"fmt"
)

// ErrUnsupported is the standard library's unsupported-operation sentinel.
var ErrUnsupported = stderrors.ErrUnsupported

// New returns an error with text and a stack captured at the call site.
func New(text string) error {
	return &plainError{trace: capture(stderrors.New(text).Error())}
}

// Errorf formats an error using fmt.Errorf, preserving its message and unwrap
// shape, and captures a stack at the call site. Use %w to wrap causes.
func Errorf(format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	t := capture(err.Error())
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		return &multiError{trace: t, children: e.Unwrap()}
	case interface{ Unwrap() error }:
		return &singleError{trace: t, child: e.Unwrap()}
	default:
		return &plainError{trace: t}
	}
}

// Join joins non-nil errors using errors.Join and captures a stack at the call
// site. It returns nil if every argument is nil.
func Join(errs ...error) error {
	err := stderrors.Join(errs...)
	if err == nil {
		return nil
	}
	return &multiError{
		trace:    capture(""),
		children: err.(interface{ Unwrap() []error }).Unwrap(),
		joined:   err,
	}
}

// Is reports whether any error in err's tree matches target, using errors.Is.
func Is(err, target error) bool { return stderrors.Is(err, target) }

// As finds the first error in err's tree matching target, using errors.As.
func As(err error, target any) bool { return stderrors.As(err, target) }

// AsType finds the first error in err's tree matching E, using errors.AsType.
func AsType[E error](err error) (E, bool) { return stderrors.AsType[E](err) }

// Unwrap returns err's single cause, using errors.Unwrap. It returns nil for
// errors that unwrap to multiple causes, including Join's results.
func Unwrap(err error) error { return stderrors.Unwrap(err) }

type plainError struct{ trace }

func (e *plainError) Format(s fmt.State, verb rune) {
	format(s, verb, e.message, e)
}

type singleError struct {
	trace
	child error
}

func (e *singleError) Unwrap() error { return e.child }

func (e *singleError) Format(s fmt.State, verb rune) {
	format(s, verb, e.message, e)
}

type multiError struct {
	trace
	children []error
	joined   error
}

func (e *multiError) Unwrap() []error { return e.children }

func (e *multiError) Error() string {
	if e.joined != nil {
		return e.joined.Error()
	}
	return e.message
}

func (e *multiError) Format(s fmt.State, verb rune) {
	format(s, verb, e.Error(), e)
}
