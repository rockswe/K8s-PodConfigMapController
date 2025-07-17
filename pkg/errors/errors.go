package errors

import (
	"fmt"
	"strings"
)

// ErrorType represents the type of error
type ErrorType string

const (
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeAPI represents Kubernetes API errors
	ErrorTypeAPI ErrorType = "api"
	// ErrorTypeReconciliation represents reconciliation errors
	ErrorTypeReconciliation ErrorType = "reconciliation"
	// ErrorTypeConversion represents conversion errors
	ErrorTypeConversion ErrorType = "conversion"
	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeConfiguration represents configuration errors
	ErrorTypeConfiguration ErrorType = "configuration"
	// ErrorTypeInternal represents internal errors
	ErrorTypeInternal ErrorType = "internal"
)

// ControllerError represents a controller-specific error with context
type ControllerError struct {
	Type      ErrorType
	Operation string
	Resource  string
	Message   string
	Cause     error
	Details   map[string]interface{}
}

// Error implements the error interface
func (e *ControllerError) Error() string {
	var parts []string

	if e.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%s", e.Type))
	}

	if e.Operation != "" {
		parts = append(parts, fmt.Sprintf("operation=%s", e.Operation))
	}

	if e.Resource != "" {
		parts = append(parts, fmt.Sprintf("resource=%s", e.Resource))
	}

	if e.Message != "" {
		parts = append(parts, fmt.Sprintf("message=%s", e.Message))
	}

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause=%v", e.Cause))
	}

	return strings.Join(parts, " ")
}

// Unwrap returns the underlying error
func (e *ControllerError) Unwrap() error {
	return e.Cause
}

// WithDetail adds a detail to the error
func (e *ControllerError) WithDetail(key string, value interface{}) *ControllerError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// NewValidationError creates a new validation error
func NewValidationError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeValidation,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewAPIError creates a new API error
func NewAPIError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeAPI,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewReconciliationError creates a new reconciliation error
func NewReconciliationError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeReconciliation,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewConversionError creates a new conversion error
func NewConversionError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeConversion,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeTimeout,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeConfiguration,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// NewInternalError creates a new internal error
func NewInternalError(operation, resource, message string, cause error) *ControllerError {
	return &ControllerError{
		Type:      ErrorTypeInternal,
		Operation: operation,
		Resource:  resource,
		Message:   message,
		Cause:     cause,
	}
}

// IsType checks if an error is of a specific type
func IsType(err error, errorType ErrorType) bool {
	if controllerErr, ok := err.(*ControllerError); ok {
		return controllerErr.Type == errorType
	}
	return false
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	return IsType(err, ErrorTypeValidation)
}

// IsAPIError checks if an error is an API error
func IsAPIError(err error) bool {
	return IsType(err, ErrorTypeAPI)
}

// IsReconciliationError checks if an error is a reconciliation error
func IsReconciliationError(err error) bool {
	return IsType(err, ErrorTypeReconciliation)
}

// IsConversionError checks if an error is a conversion error
func IsConversionError(err error) bool {
	return IsType(err, ErrorTypeConversion)
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	return IsType(err, ErrorTypeTimeout)
}

// IsConfigurationError checks if an error is a configuration error
func IsConfigurationError(err error) bool {
	return IsType(err, ErrorTypeConfiguration)
}

// IsInternalError checks if an error is an internal error
func IsInternalError(err error) bool {
	return IsType(err, ErrorTypeInternal)
}

// GetErrorType returns the error type if it's a ControllerError
func GetErrorType(err error) ErrorType {
	if controllerErr, ok := err.(*ControllerError); ok {
		return controllerErr.Type
	}
	return ""
}

// GetErrorDetails returns the error details if it's a ControllerError
func GetErrorDetails(err error) map[string]interface{} {
	if controllerErr, ok := err.(*ControllerError); ok {
		return controllerErr.Details
	}
	return nil
}

// AggregateErrors combines multiple errors into a single error
type AggregateErrors struct {
	Errors []error
}

// Error implements the error interface
func (ae *AggregateErrors) Error() string {
	if len(ae.Errors) == 0 {
		return ""
	}

	if len(ae.Errors) == 1 {
		return ae.Errors[0].Error()
	}

	var messages []string
	for _, err := range ae.Errors {
		messages = append(messages, err.Error())
	}

	return fmt.Sprintf("multiple errors: [%s]", strings.Join(messages, "; "))
}

// Add adds an error to the aggregate
func (ae *AggregateErrors) Add(err error) {
	if err != nil {
		ae.Errors = append(ae.Errors, err)
	}
}

// HasErrors returns true if there are any errors
func (ae *AggregateErrors) HasErrors() bool {
	return len(ae.Errors) > 0
}

// ToError returns the aggregate as an error if there are errors, nil otherwise
func (ae *AggregateErrors) ToError() error {
	if ae.HasErrors() {
		return ae
	}
	return nil
}

// NewAggregateErrors creates a new aggregate errors instance
func NewAggregateErrors() *AggregateErrors {
	return &AggregateErrors{
		Errors: make([]error, 0),
	}
}
