package helper

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// StatusSuccess dan StatusError adalah nilai field `status` pada envelope.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// SuccessResponse adalah bentuk baku response sukses.
type SuccessResponse struct {
	Status    string `json:"status"`
	Msg       string `json:"msg"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

func RequestID(c *fiber.Ctx) string {
	if value, ok := c.Locals(RequestIDKey).(string); ok && value != "" {
		return value
	}
	return c.Get("X-Request-ID")
}

func JSON(c *fiber.Ctx, status int, data any) error {
	return c.Status(status).JSON(SuccessResponse{
		Status:    StatusSuccess,
		Msg:       successMessage(status),
		Data:      data,
		RequestID: RequestID(c),
	})
}

// successMessage memberi pesan default sesuai status HTTP sukses.
func successMessage(status int) string {
	switch status {
	case fiber.StatusCreated:
		return "Berhasil dibuat."
	case fiber.StatusAccepted:
		return "Permintaan diterima."
	default:
		return "Berhasil."
	}
}

func OK(c *fiber.Ctx, data any) error {
	return JSON(c, fiber.StatusOK, data)
}

func Created(c *fiber.Ctx, data any) error {
	return JSON(c, fiber.StatusCreated, data)
}

func Accepted(c *fiber.Ctx, data any) error {
	return JSON(c, fiber.StatusAccepted, data)
}

func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// WriteCSV menulis CSV dengan sanitasi formula injection pada setiap sel.
func WriteCSV(c *fiber.Ctx, filename string, header []string, rows [][]string) error {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if err := writer.Write(header); err != nil {
		return WrapAppError(err, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyusun CSV.")
	}

	for _, row := range rows {
		clean := make([]string, len(row))
		for i, value := range row {
			clean[i] = sanitizeCSV(value)
		}
		if err := writer.Write(clean); err != nil {
			return WrapAppError(err, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyusun CSV.")
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return WrapAppError(err, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Gagal menyusun CSV.")
	}

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))

	return c.Status(fiber.StatusOK).Send(buf.Bytes())
}

func sanitizeCSV(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	default:
		return value
	}
}
