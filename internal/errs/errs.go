// Package errs wraps github.com/samber/oops for structured pipeline errors.
//
// Handlers return plain error values; oops adds stack traces and key/value context
// (handler id, collection name, …). Messages never travel through coll.Stream.
package errs

import "github.com/samber/oops"

// New creates a new error with a message (no underlying cause).
func New(msg string) error {
	return oops.Errorf("%s", msg)
}

// Wrap annotates err with msg and optional key/value pairs for oops context.
func Wrap(err error, msg string, attrs ...any) error {
	if err == nil {
		return nil
	}
	b := oops.With(err)
	for i := 0; i+1 < len(attrs); i += 2 {
		b = b.With(attrs[i], attrs[i+1])
	}
	return b.Wrapf(err, "%s", msg)
}

// Wrapf is Wrap with a formatted message.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, format, args...)
}

// Stacktrace returns the oops stack trace string, or "" if err is not oops.
func Stacktrace(err error) string {
	if o, ok := oops.AsOops(err); ok {
		return o.Stacktrace()
	}
	return ""
}
