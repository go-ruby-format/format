package format

// Error is the error returned by Sprintf/Format. Class names the Ruby exception
// class MRI would raise ("ArgumentError", "KeyError", "TypeError") so a host
// (rbgo) can re-raise the matching Ruby exception; Message is MRI's message
// text. Error() renders "Class: Message" for standalone Go callers.
type Error struct {
	Class   string
	Message string
}

// Error implements the error interface as "Class: Message".
func (e *Error) Error() string { return e.Class + ": " + e.Message }

// argError is a sentinel carried by parseRubyInteger/parseRubyFloat for an
// invalid String() coercion; it is promoted to an ArgumentError by the caller.
type argError struct{ msg string }

func (e *argError) Error() string { return e.msg }

// argumentError builds an ArgumentError.
func argumentError(msg string) *Error { return &Error{Class: "ArgumentError", Message: msg} }

// keyError builds a KeyError.
func keyError(msg string) *Error { return &Error{Class: "KeyError", Message: msg} }

// typeError builds a TypeError.
func typeError(msg string) *Error { return &Error{Class: "TypeError", Message: msg} }
