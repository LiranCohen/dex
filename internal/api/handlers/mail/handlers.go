// Package mail provides HTTP handlers for mail and calendar operations.
// These handlers expose the Central mail/calendar proxy API to the
// local frontend and AI sessions via the HQ API server.
package mail

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/lirancohen/dex/internal/api/core"
	"github.com/lirancohen/dex/internal/centralmail"
)

// Handler handles mail and calendar HTTP requests.
type Handler struct {
	deps   *core.Deps
	client *centralmail.Client
}

// Config contains configuration for the mail handler.
type Config struct {
	CentralURL  string
	TunnelToken string
}

// New creates a new mail/calendar handler.
func New(deps *core.Deps, cfg Config) *Handler {
	return &Handler{
		deps:   deps,
		client: centralmail.NewClient(cfg.CentralURL, cfg.TunnelToken),
	}
}

// RegisterRoutes registers mail and calendar routes on the given group.
func (h *Handler) RegisterRoutes(g *echo.Group) {
	if h.client == nil {
		return // Mail not configured — don't register routes
	}

	// Mail routes
	mail := g.Group("/mail")
	mail.GET("/folders", h.GetFolders)
	mail.POST("/send", h.SendEmail)
	mail.GET("/messages", h.ListEmails)
	mail.GET("/search", h.SearchEmails)
	mail.GET("/messages/:folderId/:messageId/content", h.GetEmailContent)
	mail.POST("/messages/mark-read", h.MarkAsRead)
	mail.POST("/messages/mark-unread", h.MarkAsUnread)
	mail.POST("/messages/:messageId/move", h.MoveEmail)
	mail.DELETE("/messages/:folderId/:messageId", h.DeleteEmail)
	mail.POST("/messages/:messageId/reply", h.ReplyToEmail)
	mail.GET("/messages/:folderId/:messageId/attachments", h.GetAttachmentInfo)
	mail.GET("/messages/:folderId/:messageId/attachments/:attachmentId", h.GetAttachmentContent)
	mail.GET("/notifications", h.GetNotifications)

	// Calendar routes
	cal := g.Group("/calendar")
	cal.GET("/calendars", h.GetCalendars)
	cal.GET("/calendars/:calendarUid", h.GetCalendar)
	cal.GET("/calendars/:calendarUid/events", h.ListEvents)
	cal.POST("/calendars/:calendarUid/events", h.CreateEvent)
	cal.PUT("/calendars/:calendarUid/events/:eventUid", h.UpdateEvent)
	cal.DELETE("/calendars/:calendarUid/events/:eventUid", h.DeleteEvent)
}

// --- Mail Handlers ---

// GetFolders returns all mail folders.
func (h *Handler) GetFolders(c echo.Context) error {
	folders, err := h.client.GetFolders()
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get folders: " + err.Error()})
	}
	return c.JSON(http.StatusOK, folders)
}

// SendEmail sends an email.
func (h *Handler) SendEmail(c echo.Context) error {
	var req centralmail.SendEmailRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.ToAddress == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "toAddress is required"})
	}

	resp, err := h.client.SendEmail(req)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to send email: " + err.Error()})
	}
	return c.JSON(http.StatusOK, resp)
}

// ListEmails lists emails with optional filtering.
func (h *Handler) ListEmails(c echo.Context) error {
	opts := centralmail.ListEmailOpts{
		FolderID:  c.QueryParam("folderId"),
		Status:    c.QueryParam("status"),
		SortBy:    c.QueryParam("sortBy"),
		SortOrder: c.QueryParam("sortOrder"),
	}
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Limit = n
		}
	}
	if v := c.QueryParam("start"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Start = n
		}
	}

	emails, err := h.client.ListEmails(opts)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to list emails: " + err.Error()})
	}
	return c.JSON(http.StatusOK, emails)
}

// SearchEmails searches emails.
func (h *Handler) SearchEmails(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "q (search query) is required"})
	}

	limit := 20
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	emails, err := h.client.SearchEmails(query, limit)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to search emails: " + err.Error()})
	}
	return c.JSON(http.StatusOK, emails)
}

// GetEmailContent returns full email content.
func (h *Handler) GetEmailContent(c echo.Context) error {
	content, err := h.client.GetEmailContent(c.Param("folderId"), c.Param("messageId"))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get email: " + err.Error()})
	}
	return c.JSON(http.StatusOK, content)
}

// MarkAsRead marks messages as read.
func (h *Handler) MarkAsRead(c echo.Context) error {
	var req struct {
		MessageIDs []string `json:"messageIds"`
	}
	if err := c.Bind(&req); err != nil || len(req.MessageIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "messageIds is required"})
	}
	if err := h.client.MarkAsRead(req.MessageIDs); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to mark as read: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// MarkAsUnread marks messages as unread.
func (h *Handler) MarkAsUnread(c echo.Context) error {
	var req struct {
		MessageIDs []string `json:"messageIds"`
	}
	if err := c.Bind(&req); err != nil || len(req.MessageIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "messageIds is required"})
	}
	if err := h.client.MarkAsUnread(req.MessageIDs); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to mark as unread: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// MoveEmail moves an email to a different folder.
func (h *Handler) MoveEmail(c echo.Context) error {
	var req struct {
		DestFolderID string `json:"destFolderId"`
	}
	if err := c.Bind(&req); err != nil || req.DestFolderID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "destFolderId is required"})
	}
	if err := h.client.MoveEmail(c.Param("messageId"), req.DestFolderID); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to move email: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// DeleteEmail deletes an email.
func (h *Handler) DeleteEmail(c echo.Context) error {
	if err := h.client.DeleteEmail(c.Param("folderId"), c.Param("messageId")); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to delete email: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ReplyToEmail replies to an existing email.
func (h *Handler) ReplyToEmail(c echo.Context) error {
	var req centralmail.ReplyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "content is required"})
	}
	if err := h.client.ReplyToEmail(c.Param("messageId"), req); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to reply: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// GetAttachmentInfo returns attachment metadata.
func (h *Handler) GetAttachmentInfo(c echo.Context) error {
	info, err := h.client.GetAttachmentInfo(c.Param("folderId"), c.Param("messageId"))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get attachments: " + err.Error()})
	}
	return c.JSON(http.StatusOK, info)
}

// GetAttachmentContent downloads an attachment.
func (h *Handler) GetAttachmentContent(c echo.Context) error {
	content, err := h.client.GetAttachmentContent(c.Param("folderId"), c.Param("messageId"), c.Param("attachmentId"))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get attachment: " + err.Error()})
	}
	return c.JSON(http.StatusOK, content)
}

// GetNotifications checks for new mail notifications.
func (h *Handler) GetNotifications(c echo.Context) error {
	notif, err := h.client.GetNotifications()
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get notifications: " + err.Error()})
	}
	return c.JSON(http.StatusOK, notif)
}

// --- Calendar Handlers ---

// GetCalendars returns all calendars.
func (h *Handler) GetCalendars(c echo.Context) error {
	calendars, err := h.client.GetCalendars()
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get calendars: " + err.Error()})
	}
	return c.JSON(http.StatusOK, calendars)
}

// GetCalendar returns a specific calendar.
func (h *Handler) GetCalendar(c echo.Context) error {
	cal, err := h.client.GetCalendar(c.Param("calendarUid"))
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to get calendar: " + err.Error()})
	}
	return c.JSON(http.StatusOK, cal)
}

// ListEvents returns events in a date range.
func (h *Handler) ListEvents(c echo.Context) error {
	start := c.QueryParam("start")
	end := c.QueryParam("end")
	if start == "" || end == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "start and end query parameters are required (RFC3339)"})
	}

	events, err := h.client.ListEvents(c.Param("calendarUid"), start, end)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to list events: " + err.Error()})
	}
	return c.JSON(http.StatusOK, events)
}

// CreateEvent creates a new calendar event.
func (h *Handler) CreateEvent(c echo.Context) error {
	var req centralmail.CreateEventRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}

	event, err := h.client.CreateEvent(c.Param("calendarUid"), req)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to create event: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, event)
}

// UpdateEvent updates a calendar event.
func (h *Handler) UpdateEvent(c echo.Context) error {
	var req centralmail.UpdateEventRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	event, err := h.client.UpdateEvent(c.Param("calendarUid"), c.Param("eventUid"), req)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to update event: " + err.Error()})
	}
	return c.JSON(http.StatusOK, event)
}

// DeleteEvent deletes a calendar event.
func (h *Handler) DeleteEvent(c echo.Context) error {
	if err := h.client.DeleteEvent(c.Param("calendarUid"), c.Param("eventUid")); err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Failed to delete event: " + err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}
