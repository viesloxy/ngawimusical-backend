package helper

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// RequestIDKey adalah kunci Locals dan header untuk request id.
const RequestIDKey = "request_id"

// AppError adalah error bisnis yang membawa kode, status HTTP, dan field yang gagal.
type AppError struct {
	Status    int
	Code      string
	Message   string
	Fields    map[string]string
	Retryable bool
	Cause     error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func NewAppError(status int, code, message string) *AppError {
	return &AppError{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

func WrapAppError(err error, status int, code, message string) *AppError {
	return &AppError{
		Status:  status,
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

func (e *AppError) WithFields(fields map[string]string) *AppError {
	e.Fields = fields
	return e
}

func (e *AppError) WithRetryable(retryable bool) *AppError {
	e.Retryable = retryable
	return e
}

func NewValidationError(fields map[string]string) *AppError {
	return NewAppError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Nilai field tidak valid.").WithFields(fields)
}

func Unauthenticated() *AppError {
	return NewAppError(http.StatusUnauthorized, "UNAUTHENTICATED", "Autentikasi diperlukan.")
}

func Forbidden() *AppError {
	return NewAppError(http.StatusForbidden, "FORBIDDEN", "Tidak punya hak akses.")
}

func NotFound() *AppError {
	return NewAppError(http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource tidak ditemukan.")
}

func Conflict(code, message string) *AppError {
	return NewAppError(http.StatusConflict, code, message)
}

func Unavailable(code, message string) *AppError {
	return NewAppError(http.StatusServiceUnavailable, code, message).WithRetryable(true)
}

// ErrorResponse adalah bentuk baku response error.
type ErrorResponse struct {
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id"`
}

type ErrorDetail struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	Retryable bool              `json:"retryable"`
}

// ErrorHandler memetakan seluruh error ke format response yang sama.
func ErrorHandler(c *fiber.Ctx, err error) error {
	requestID := RequestID(c)

	var appErr *AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Status).JSON(ErrorResponse{
			Error: ErrorDetail{
				Code:      appErr.Code,
				Message:   appErr.Message,
				Fields:    appErr.Fields,
				Retryable: appErr.Retryable,
			},
			RequestID: requestID,
		})
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(ErrorResponse{
			Error: ErrorDetail{
				Code:    codeForStatus(fiberErr.Code),
				Message: messageForStatus(fiberErr.Code),
			},
			RequestID: requestID,
		})
	}

	slog.Error("kesalahan internal", "request_id", requestID, "error", err)

	return c.Status(http.StatusInternalServerError).JSON(ErrorResponse{
		Error: ErrorDetail{
			Code:    "INTERNAL_ERROR",
			Message: "Terjadi kesalahan pada server.",
		},
		RequestID: requestID,
	})
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "MALFORMED_JSON"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "RESOURCE_NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusNotAcceptable:
		return "NOT_ACCEPTABLE"
	case http.StatusUnsupportedMediaType:
		return "UNSUPPORTED_MEDIA_TYPE"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusServiceUnavailable:
		return "SYSTEM_BUSY"
	default:
		return "INTERNAL_ERROR"
	}
}

func messageForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "Format request tidak valid."
	case http.StatusUnauthorized:
		return "Autentikasi diperlukan."
	case http.StatusForbidden:
		return "Tidak punya hak akses."
	case http.StatusNotFound:
		return "Resource tidak ditemukan."
	case http.StatusMethodNotAllowed:
		return "Method tidak didukung."
	case http.StatusNotAcceptable:
		return "Format response tidak didukung."
	case http.StatusUnsupportedMediaType:
		return "Content type tidak didukung."
	case http.StatusConflict:
		return "Terjadi konflik data."
	case http.StatusTooManyRequests:
		return "Terlalu banyak request."
	case http.StatusServiceUnavailable:
		return "Sistem sedang sibuk."
	default:
		return "Terjadi kesalahan pada server."
	}
}
