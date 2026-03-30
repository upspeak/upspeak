package core

import "fmt"

// ErrorNotFound indicates that a requested resource was not found.
type ErrorNotFound struct {
	resource string
	msg      string
}

func (e *ErrorNotFound) Error() string {
	return fmt.Sprintf("not found: could not find %s: %s", e.resource, e.msg)
}

// NewErrorNotFound creates a new ErrorNotFound.
func NewErrorNotFound(resource, msg string) *ErrorNotFound {
	return &ErrorNotFound{resource: resource, msg: msg}
}

// ErrorUnmarshal indicates a JSON unmarshalling failure.
type ErrorUnmarshal struct {
	msg string
}

func (e *ErrorUnmarshal) Error() string {
	return fmt.Sprintf("unmarshal error: %s", e.msg)
}

// ErrorSave indicates a failure persisting an entity.
type ErrorSave struct {
	msg string
}

func (e *ErrorSave) Error() string {
	return fmt.Sprintf("save error: %s", e.msg)
}

// ErrorDelete indicates a failure deleting an entity.
type ErrorDelete struct {
	msg string
}

func (e *ErrorDelete) Error() string {
	return fmt.Sprintf("delete error: %s", e.msg)
}

// ErrorEventCreation indicates a failure creating an event.
type ErrorEventCreation struct {
	msg string
}

func (e *ErrorEventCreation) Error() string {
	return fmt.Sprintf("event creation error: %s", e.msg)
}

// ErrorPublish indicates a failure publishing an event.
type ErrorPublish struct {
	msg string
}

func (e *ErrorPublish) Error() string {
	return fmt.Sprintf("publish error: %s", e.msg)
}

// ErrorUnknownEventType indicates that an event type is not recognised.
type ErrorUnknownEventType struct {
	eventType string
}

func (e *ErrorUnknownEventType) Error() string {
	return fmt.Sprintf("unknown event type: %s", e.eventType)
}

// ErrorSlugConflict indicates that a repository slug is already in use.
type ErrorSlugConflict struct {
	Slug string
}

func (e *ErrorSlugConflict) Error() string {
	return fmt.Sprintf("slug conflict: %s is already in use", e.Slug)
}

// ErrorSlugRedirect indicates that a slug has been renamed and should redirect.
type ErrorSlugRedirect struct {
	OldSlug string
	NewSlug string
	RepoID  string
}

func (e *ErrorSlugRedirect) Error() string {
	return fmt.Sprintf("slug redirect: %s has been renamed to %s", e.OldSlug, e.NewSlug)
}

// ErrorValidation indicates that a request failed validation.
type ErrorValidation struct {
	Field   string
	Message string
}

func (e *ErrorValidation) Error() string {
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}
