package service

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

// EventService menangani HTTP request/response untuk endpoint event.
type EventService struct {
	uc           *EventUsecase
	cursorSecret []byte
}

// NewEventService membuat instance baru EventService.
func NewEventService(uc *EventUsecase, cursorSecret []byte) *EventService {
	return &EventService{uc: uc, cursorSecret: cursorSecret}
}

// Register mendaftarkan seluruh endpoint event ke router Fiber.
// Sesuai dengan kontrak tim (ADR-02), setiap service mengekspor Register.
func (s *EventService) Register(router fiber.Router) {
	events := router.Group("/events")

	// Endpoint publik (pengunjung)
	events.Get("", s.ListPublishedEvents)
	events.Get("/:id", s.GetEvent)
	events.Get("/:id/tiers", s.ListTiers)

	// Endpoint mutasi organizer
	events.Post("", s.CreateEvent)
	events.Patch("/:id", s.UpdateEvent)
	events.Post("/:id/tiers", s.CreateTier)
	events.Post("/:id/publish", s.PublishEvent)
	events.Post("/:id/pause", s.PauseSale)
	events.Post("/:id/resume", s.ResumeSale)
	events.Post("/:id/close", s.CloseSale)
	events.Post("/:id/cancel", s.CancelEvent)
	events.Post("/:id/complete", s.CompleteEvent)

	// Endpoint tier update
	router.Patch("/tiers/:id", s.UpdateTier)
}

// CreateEvent menangani POST /api/v1/events.
func (s *EventService) CreateEvent(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	if actor.Role != "organizer" && actor.Role != "admin" {
		return helper.Forbidden()
	}

	var req model.CreateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request tidak valid.")
	}

	requestID := helper.RequestID(c)
	event, err := s.uc.CreateEvent(c.UserContext(), actor.UserID, req, requestID)
	if err != nil {
		return err
	}

	return helper.Created(c, event)
}

// GetEvent menangani GET /api/v1/events/:id.
func (s *EventService) GetEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	event, err := s.uc.GetEvent(c.UserContext(), eventID)
	if err != nil {
		return err
	}

	// Jika event bukan PUBLISHED, hanya organizer pemilik atau admin yang boleh melihat
	if event.Status != model.EventStatusPublished {
		actor, ok := middleware.GetActor(c)
		if !ok || (actor.UserID != event.OrganizerID && actor.Role != "admin") {
			return helper.NotFound()
		}
	}

	return helper.OK(c, event)
}

// ListPublishedEvents menangani GET /api/v1/events.
func (s *EventService) ListPublishedEvents(c *fiber.Ctx) error {
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

	events, err := s.uc.ListPublishedEvents(c.UserContext(), cursor, limit)
	if err != nil {
		return err
	}

	responses := make([]model.EventListResponse, len(events))
	for i, e := range events {
		responses[i] = ToEventListResponse(e)
	}

	result := fiber.Map{
		"items": responses,
	}

	if len(events) == limit {
		last := events[len(events)-1]
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

// UpdateEvent menangani PATCH /api/v1/events/:id.
func (s *EventService) UpdateEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	var req model.UpdateEventRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request tidak valid.")
	}

	requestID := helper.RequestID(c)
	event, err := s.uc.UpdateEvent(c.UserContext(), eventID, actor.UserID, req, requestID)
	if err != nil {
		return err
	}

	return helper.OK(c, event)
}

// CreateTier menangani POST /api/v1/events/:id/tiers.
func (s *EventService) CreateTier(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	var req model.CreateTierRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request tidak valid.")
	}

	requestID := helper.RequestID(c)
	tier, err := s.uc.CreateTier(c.UserContext(), eventID, actor.UserID, req, requestID)
	if err != nil {
		return err
	}

	return helper.Created(c, tier)
}

// UpdateTier menangani PATCH /api/v1/tiers/:id.
func (s *EventService) UpdateTier(c *fiber.Ctx) error {
	tierID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID tier tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	var req model.UpdateTierRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request tidak valid.")
	}

	requestID := helper.RequestID(c)
	tier, err := s.uc.UpdateTier(c.UserContext(), tierID, actor.UserID, req, requestID)
	if err != nil {
		return err
	}

	return helper.OK(c, tier)
}

// ListTiers menangani GET /api/v1/events/:id/tiers.
func (s *EventService) ListTiers(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	tiers, err := s.uc.ListTiers(c.UserContext(), eventID)
	if err != nil {
		return err
	}

	responses := make([]model.TierResponse, len(tiers))
	for i, t := range tiers {
		responses[i] = ToTierResponse(t)
	}

	return helper.OK(c, responses)
}

// PublishEvent menangani POST /api/v1/events/:id/publish.
func (s *EventService) PublishEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	requestID := helper.RequestID(c)
	if err := s.uc.PublishEvent(c.UserContext(), eventID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Event berhasil dipublikasikan."})
}

// PauseSale menangani POST /api/v1/events/:id/pause.
func (s *EventService) PauseSale(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	requestID := helper.RequestID(c)
	if err := s.uc.PauseSale(c.UserContext(), eventID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Penjualan dijeda."})
}

// ResumeSale menangani POST /api/v1/events/:id/resume.
func (s *EventService) ResumeSale(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	requestID := helper.RequestID(c)
	if err := s.uc.ResumeSale(c.UserContext(), eventID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Penjualan dilanjutkan."})
}

// CloseSale menangani POST /api/v1/events/:id/close.
func (s *EventService) CloseSale(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	requestID := helper.RequestID(c)
	if err := s.uc.CloseSale(c.UserContext(), eventID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Penjualan ditutup."})
}

// CancelEvent menangani POST /api/v1/events/:id/cancel.
func (s *EventService) CancelEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	var body struct {
		Reason string `json:"reason" validate:"required,min=3,max=500"`
	}
	if err := c.BodyParser(&body); err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(body); err != nil {
		return err
	}

	requestID := helper.RequestID(c)
	if err := s.uc.CancelEvent(c.UserContext(), eventID, actor.UserID, body.Reason, requestID); err != nil {
		return err
	}

	return helper.Accepted(c, fiber.Map{"message": "Pembatalan event diterima."})
}

// CompleteEvent menangani POST /api/v1/events/:id/complete.
func (s *EventService) CompleteEvent(c *fiber.Ctx) error {
	eventID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return helper.NewAppError(400, "MALFORMED_JSON", "ID event tidak valid.")
	}

	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	requestID := helper.RequestID(c)
	if err := s.uc.CompleteEvent(c.UserContext(), eventID, actor.UserID, requestID); err != nil {
		return err
	}

	return helper.OK(c, fiber.Map{"message": "Event ditandai selesai."})
}
