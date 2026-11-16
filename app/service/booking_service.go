package service

import (
	"encoding/json"
	"github.com/viesloxy/ngawimusical-backend/app/usecase"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

// BookingService menangani HTTP request/response untuk endpoint booking.
type BookingService struct {
	uc           *usecase.BookingUsecase
	cursorSecret []byte
}

// NewBookingService membuat instance baru BookingService.
func NewBookingService(uc *usecase.BookingUsecase, cursorSecret []byte) *BookingService {
	return &BookingService{uc: uc, cursorSecret: cursorSecret}
}

// Register mendaftarkan seluruh endpoint booking ke router Fiber.
// Sesuai dengan kontrak tim (ADR-02), setiap service mengekspor Register.
func (s *BookingService) Register(router fiber.Router) {
	bookings := router.Group("/bookings")

	// Endpoint reservasi hold & manajemen booking attendee
	bookings.Post("", s.CreateHold)
	bookings.Get("", s.ListBookings)
	bookings.Get("/:id", s.GetBooking)
	bookings.Post("/:id/cancel", s.CancelHold)
}

// CreateHold menangani POST /api/v1/bookings.
func (s *BookingService) CreateHold(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		return helper.NewValidationError(map[string]string{
			"idempotency_key": "Header Idempotency-Key wajib disertakan.",
		})
	}
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 128 {
		return helper.NewValidationError(map[string]string{
			"idempotency_key": "Panjang Idempotency-Key harus antara 16 dan 128 karakter.",
		})
	}

	var req model.CreateBookingRequest
	bodyBytes := c.Body()
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request JSON tidak valid.")
	}

	if err := helper.Validate(req); err != nil {
		return err
	}

	eventID, err := strconv.ParseInt(req.EventID, 10, 64)
	if err != nil {
		return helper.NewValidationError(map[string]string{"event_id": "Event ID tidak valid."})
	}

	tierID, err := strconv.ParseInt(req.TierID, 10, 64)
	if err != nil {
		return helper.NewValidationError(map[string]string{"tier_id": "Tier ID tidak valid."})
	}

	requestID := helper.RequestID(c)

	// Kanonisasi payload untuk idempotency hash
	canonicalPayload, _ := json.Marshal(map[string]any{
		"event_id": eventID,
		"tier_id":  tierID,
		"quantity": req.Quantity,
	})

	internalReq := usecase.CreateHoldRequestInternal{
		UserID:         actor.UserID,
		EventID:        eventID,
		TierID:         tierID,
		Quantity:       req.Quantity,
		IdempotencyKey: idempotencyKey,
		RequestID:      requestID,
		PayloadBytes:   canonicalPayload,
	}

	resp, err := s.uc.CreateHold(c.UserContext(), internalReq)
	if err != nil {
		return err
	}

	return helper.Created(c, resp)
}

// GetBooking menangani GET /api/v1/bookings/:id.
func (s *BookingService) GetBooking(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	bookingID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID booking tidak valid.")
	}

	resp, err := s.uc.GetBooking(c.UserContext(), bookingID, actor.UserID, actor.Role)
	if err != nil {
		return err
	}

	return helper.OK(c, resp)
}

// ListBookings menangani GET /api/v1/bookings.
func (s *BookingService) ListBookings(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var cursor *helper.Cursor
	if raw := c.Query("cursor"); raw != "" {
		decoded, err := helper.DecodeCursor(s.cursorSecret, raw)
		if err != nil {
			return helper.NewAppError(400, "MALFORMED_JSON", "Cursor tidak valid.")
		}
		cursor = &decoded
	}

	bookings, err := s.uc.ListUserBookings(c.UserContext(), actor.UserID, cursor, limit)
	if err != nil {
		return err
	}

	result := fiber.Map{
		"items": bookings,
	}

	if len(bookings) == limit {
		last := bookings[len(bookings)-1]
		nextCursor, err := helper.EncodeCursor(s.cursorSecret, helper.Cursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		})
		if err == nil {
			result["next_cursor"] = nextCursor
		}
	}

	return helper.OK(c, result)
}

// CancelHold menangani POST /api/v1/bookings/:id/cancel.
func (s *BookingService) CancelHold(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	bookingID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID booking tidak valid.")
	}

	requestID := helper.RequestID(c)
	if err := s.uc.CancelHold(c.UserContext(), bookingID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Reservasi berhasil dibatalkan."})
}
