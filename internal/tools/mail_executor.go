package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/lirancohen/dex/internal/centralmail"
)

// MailExecutor executes mail and calendar tools via the Central mail client.
// It is wired into the AI session alongside the base Executor and workflow Executor.
type MailExecutor struct {
	client *centralmail.Client
}

// NewMailExecutor creates a new mail tool executor.
// Returns nil if client is nil (mail not configured).
func NewMailExecutor(client *centralmail.Client) *MailExecutor {
	if client == nil {
		return nil
	}
	return &MailExecutor{client: client}
}

// Execute dispatches a mail/calendar tool call and returns the result.
func (e *MailExecutor) Execute(toolName string, input map[string]any) Result {
	start := time.Now()

	var result Result
	switch toolName {
	// Mail tools
	case "mail_list_folders":
		result = e.listFolders()
	case "mail_list_messages":
		result = e.listMessages(input)
	case "mail_search":
		result = e.searchMail(input)
	case "mail_read":
		result = e.readMail(input)
	case "mail_send":
		result = e.sendMail(input)
	case "mail_reply":
		result = e.replyMail(input)
	case "mail_delete":
		result = e.deleteMail(input)
	// Calendar tools
	case "calendar_list":
		result = e.listCalendars()
	case "calendar_list_events":
		result = e.listEvents(input)
	case "calendar_create_event":
		result = e.createEvent(input)
	case "calendar_update_event":
		result = e.updateEvent(input)
	case "calendar_delete_event":
		result = e.deleteEvent(input)
	default:
		result = Result{
			Output:  fmt.Sprintf("Unknown mail/calendar tool: %s", toolName),
			IsError: true,
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// CanHandle returns true if this executor handles the given tool name.
func (e *MailExecutor) CanHandle(toolName string) bool {
	switch toolName {
	case "mail_list_folders", "mail_list_messages", "mail_search", "mail_read",
		"mail_send", "mail_reply", "mail_delete",
		"calendar_list", "calendar_list_events", "calendar_create_event",
		"calendar_update_event", "calendar_delete_event":
		return true
	}
	return false
}

// --- Mail implementations ---

func (e *MailExecutor) listFolders() Result {
	folders, err := e.client.GetFolders()
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to list folders: %v", err), IsError: true}
	}
	return e.jsonResult(folders)
}

func (e *MailExecutor) listMessages(input map[string]any) Result {
	opts := centralmail.ListEmailOpts{}
	if v, ok := input["folder_id"].(string); ok {
		opts.FolderID = v
	}
	if v, ok := input["status"].(string); ok {
		opts.Status = v
	}
	if v, ok := input["limit"].(float64); ok {
		opts.Limit = int(v)
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 50 {
		opts.Limit = 50
	}

	emails, err := e.client.ListEmails(opts)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to list messages: %v", err), IsError: true}
	}

	// Format as readable text for the AI
	if len(emails) == 0 {
		return Result{Output: "No messages found.", IsError: false}
	}

	var output string
	for i, email := range emails {
		status := "read"
		if email.Status2 == "1" {
			status = "unread"
		}
		output += fmt.Sprintf("[%d] %s | From: %s | Subject: %s | Date: %s | Status: %s | FolderID: %s | MessageID: %s\n",
			i+1, email.SentDateInGMT, email.FromAddress, email.Subject, email.SentDateInGMT, status, email.FolderID, email.MessageID)
	}
	return Result{Output: output, IsError: false}
}

func (e *MailExecutor) searchMail(input map[string]any) Result {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return Result{Output: "query is required", IsError: true}
	}

	limit := 20
	if v, ok := input["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	emails, err := e.client.SearchEmails(query, limit)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to search: %v", err), IsError: true}
	}

	if len(emails) == 0 {
		return Result{Output: fmt.Sprintf("No results found for '%s'.", query), IsError: false}
	}

	var output string
	for i, email := range emails {
		output += fmt.Sprintf("[%d] From: %s | Subject: %s | Date: %s | FolderID: %s | MessageID: %s\n",
			i+1, email.FromAddress, email.Subject, email.SentDateInGMT, email.FolderID, email.MessageID)
	}
	return Result{Output: output, IsError: false}
}

func (e *MailExecutor) readMail(input map[string]any) Result {
	folderID, ok := input["folder_id"].(string)
	if !ok || folderID == "" {
		return Result{Output: "folder_id is required", IsError: true}
	}
	messageID, ok := input["message_id"].(string)
	if !ok || messageID == "" {
		return Result{Output: "message_id is required", IsError: true}
	}

	content, err := e.client.GetEmailContent(folderID, messageID)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to read email: %v", err), IsError: true}
	}

	output := fmt.Sprintf("From: %s\nTo: %s\n", content.FromAddress, content.ToAddress)
	if content.CcAddress != "" {
		output += fmt.Sprintf("CC: %s\n", content.CcAddress)
	}
	output += fmt.Sprintf("Subject: %s\nDate: %d\n\n%s", content.Subject, content.ReceivedTime, content.Content)

	// Auto-mark as read
	_ = e.client.MarkAsRead([]string{messageID})

	return Result{Output: output, IsError: false}
}

func (e *MailExecutor) sendMail(input map[string]any) Result {
	to, ok := input["to"].(string)
	if !ok || to == "" {
		return Result{Output: "to is required", IsError: true}
	}
	subject, _ := input["subject"].(string)
	body, _ := input["body"].(string)
	if subject == "" && body == "" {
		return Result{Output: "subject or body is required", IsError: true}
	}

	req := centralmail.SendEmailRequest{
		ToAddress: to,
		Subject:   subject,
		Content:   body,
	}
	if cc, ok := input["cc"].(string); ok {
		req.CcAddress = cc
	}

	resp, err := e.client.SendEmail(req)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to send email: %v", err), IsError: true}
	}

	return Result{
		Output:  fmt.Sprintf("Email sent successfully. Message ID: %s", resp.Data.MessageID),
		IsError: false,
	}
}

func (e *MailExecutor) replyMail(input map[string]any) Result {
	messageID, ok := input["message_id"].(string)
	if !ok || messageID == "" {
		return Result{Output: "message_id is required", IsError: true}
	}
	body, ok := input["body"].(string)
	if !ok || body == "" {
		return Result{Output: "body is required", IsError: true}
	}

	req := centralmail.ReplyRequest{Content: body}
	if err := e.client.ReplyToEmail(messageID, req); err != nil {
		return Result{Output: fmt.Sprintf("Failed to reply: %v", err), IsError: true}
	}

	return Result{Output: "Reply sent successfully.", IsError: false}
}

func (e *MailExecutor) deleteMail(input map[string]any) Result {
	folderID, ok := input["folder_id"].(string)
	if !ok || folderID == "" {
		return Result{Output: "folder_id is required", IsError: true}
	}
	messageID, ok := input["message_id"].(string)
	if !ok || messageID == "" {
		return Result{Output: "message_id is required", IsError: true}
	}

	if err := e.client.DeleteEmail(folderID, messageID); err != nil {
		return Result{Output: fmt.Sprintf("Failed to delete: %v", err), IsError: true}
	}

	return Result{Output: "Email deleted successfully.", IsError: false}
}

// --- Calendar implementations ---

func (e *MailExecutor) listCalendars() Result {
	calendars, err := e.client.GetCalendars()
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to list calendars: %v", err), IsError: true}
	}
	return e.jsonResult(calendars)
}

func (e *MailExecutor) listEvents(input map[string]any) Result {
	start, ok := input["start"].(string)
	if !ok || start == "" {
		return Result{Output: "start is required (RFC3339 format)", IsError: true}
	}
	end, ok := input["end"].(string)
	if !ok || end == "" {
		return Result{Output: "end is required (RFC3339 format)", IsError: true}
	}

	calendarUID, _ := input["calendar_uid"].(string)
	if calendarUID == "" {
		// Try to get default calendar
		calendars, err := e.client.GetCalendars()
		if err != nil {
			return Result{Output: fmt.Sprintf("Failed to list calendars: %v", err), IsError: true}
		}
		for _, cal := range calendars {
			if cal.IsDefault {
				calendarUID = cal.UID
				break
			}
		}
		if calendarUID == "" && len(calendars) > 0 {
			calendarUID = calendars[0].UID
		}
		if calendarUID == "" {
			return Result{Output: "No calendars found. calendar_uid is required.", IsError: true}
		}
	}

	events, err := e.client.ListEvents(calendarUID, start, end)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to list events: %v", err), IsError: true}
	}

	if len(events) == 0 {
		return Result{Output: "No events found in the specified date range.", IsError: false}
	}

	var output string
	for i, event := range events {
		output += fmt.Sprintf("[%d] %s | %s - %s | Location: %s | UID: %s\n",
			i+1, event.Title, event.StartRaw, event.EndRaw, event.Location, event.UID)
		if event.Description != "" {
			output += fmt.Sprintf("    Description: %s\n", event.Description)
		}
	}
	return Result{Output: output, IsError: false}
}

func (e *MailExecutor) createEvent(input map[string]any) Result {
	title, ok := input["title"].(string)
	if !ok || title == "" {
		return Result{Output: "title is required", IsError: true}
	}
	start, ok := input["start"].(string)
	if !ok || start == "" {
		return Result{Output: "start is required (RFC3339 format)", IsError: true}
	}
	end, ok := input["end"].(string)
	if !ok || end == "" {
		return Result{Output: "end is required (RFC3339 format)", IsError: true}
	}

	calendarUID, _ := input["calendar_uid"].(string)
	if calendarUID == "" {
		calendars, err := e.client.GetCalendars()
		if err != nil {
			return Result{Output: fmt.Sprintf("Failed to list calendars: %v", err), IsError: true}
		}
		for _, cal := range calendars {
			if cal.IsDefault {
				calendarUID = cal.UID
				break
			}
		}
		if calendarUID == "" && len(calendars) > 0 {
			calendarUID = calendars[0].UID
		}
		if calendarUID == "" {
			return Result{Output: "No calendars found. calendar_uid is required.", IsError: true}
		}
	}

	req := centralmail.CreateEventRequest{
		Title: title,
		Start: start,
		End:   end,
	}
	if v, ok := input["description"].(string); ok {
		req.Description = v
	}
	if v, ok := input["location"].(string); ok {
		req.Location = v
	}
	if v, ok := input["all_day"].(bool); ok {
		req.IsAllDay = v
	}

	event, err := e.client.CreateEvent(calendarUID, req)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to create event: %v", err), IsError: true}
	}

	return Result{
		Output:  fmt.Sprintf("Event created: %s (UID: %s)", event.Title, event.UID),
		IsError: false,
	}
}

func (e *MailExecutor) updateEvent(input map[string]any) Result {
	calendarUID, ok := input["calendar_uid"].(string)
	if !ok || calendarUID == "" {
		return Result{Output: "calendar_uid is required", IsError: true}
	}
	eventUID, ok := input["event_uid"].(string)
	if !ok || eventUID == "" {
		return Result{Output: "event_uid is required", IsError: true}
	}

	req := centralmail.UpdateEventRequest{}
	if v, ok := input["title"].(string); ok {
		req.Title = v
	}
	if v, ok := input["description"].(string); ok {
		req.Description = v
	}
	if v, ok := input["start"].(string); ok {
		req.Start = v
	}
	if v, ok := input["end"].(string); ok {
		req.End = v
	}

	event, err := e.client.UpdateEvent(calendarUID, eventUID, req)
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to update event: %v", err), IsError: true}
	}

	return Result{
		Output:  fmt.Sprintf("Event updated: %s (UID: %s)", event.Title, event.UID),
		IsError: false,
	}
}

func (e *MailExecutor) deleteEvent(input map[string]any) Result {
	calendarUID, ok := input["calendar_uid"].(string)
	if !ok || calendarUID == "" {
		return Result{Output: "calendar_uid is required", IsError: true}
	}
	eventUID, ok := input["event_uid"].(string)
	if !ok || eventUID == "" {
		return Result{Output: "event_uid is required", IsError: true}
	}

	if err := e.client.DeleteEvent(calendarUID, eventUID); err != nil {
		return Result{Output: fmt.Sprintf("Failed to delete event: %v", err), IsError: true}
	}

	return Result{Output: "Event deleted successfully.", IsError: false}
}

// jsonResult marshals data to indented JSON for readable AI output.
func (e *MailExecutor) jsonResult(data interface{}) Result {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return Result{Output: fmt.Sprintf("Failed to format result: %v", err), IsError: true}
	}
	return Result{Output: string(b), IsError: false}
}
