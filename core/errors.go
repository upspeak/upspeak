package core

import "fmt"

type ErrorNotFound struct {
	resource string
	msg      string
}

func (e *ErrorNotFound) Error() string {
	return fmt.Sprintf("not found error: Could not find %s. Message: %s", e.resource, e.msg)
}

// Define custom error types
type ErrorUnmarshal struct {
	msg string
}

func (e *ErrorUnmarshal) Error() string {
	return fmt.Sprintf("unmarshal error: %s", e.msg)
}

type ErrorSave struct {
	msg string
}

func (e *ErrorSave) Error() string {
	return fmt.Sprintf("save error: %s", e.msg)
}

type ErrorDelete struct {
	msg string
}

func (e *ErrorDelete) Error() string {
	return fmt.Sprintf("delete error: %s", e.msg)
}

type ErrorEventCreation struct {
	msg string
}

func (e *ErrorEventCreation) Error() string {
	return fmt.Sprintf("event creation error: %s", e.msg)
}

type ErrorPublish struct {
	msg string
}

func (e *ErrorPublish) Error() string {
	return fmt.Sprintf("publish error: %s", e.msg)
}
