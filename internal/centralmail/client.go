// Package centralmail provides an HTTP client for calling Central's mail and
// calendar proxy API. HQ uses this client to relay mail/calendar operations
// through Central to Zoho Mail.
package centralmail

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client calls Central's /api/v1/hq/mail/* and /api/v1/hq/calendar/* endpoints.
type Client struct {
	centralURL  string
	tunnelToken string
	httpClient  *http.Client
}

// NewClient creates a new Central mail/calendar client.
// Returns nil if Central connection is not configured.
func NewClient(centralURL, tunnelToken string) *Client {
	if centralURL == "" || tunnelToken == "" {
		return nil
	}
	return &Client{
		centralURL:  strings.TrimSuffix(centralURL, "/"),
		tunnelToken: tunnelToken,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// --- Types ---

// Folder represents a mail folder.
type Folder struct {
	FolderID     string `json:"folderId"`
	FolderName   string `json:"folderName"`
	FolderPath   string `json:"folderPath"`
	FolderType   string `json:"folderType"`
	UnreadCount  int64  `json:"unreadCount"`
	MessageCount int64  `json:"messageCount"`
}

// EmailSummary represents a brief email summary.
type EmailSummary struct {
	MessageID     string `json:"messageId"`
	FolderID      string `json:"folderId"`
	FromAddress   string `json:"fromAddress"`
	Sender        string `json:"sender"`
	ToAddress     string `json:"toAddress"`
	CcAddress     string `json:"ccAddress"`
	Subject       string `json:"subject"`
	Summary       string `json:"summary"`
	ReceivedTime  int64  `json:"receivedTime"`
	SentDateInGMT string `json:"sentDateInGMT"`
	HasAttachment string `json:"hasAttachment"`
	Status2       string `json:"status2"`
	FlagID        string `json:"flagid"`
}

// EmailContent represents the full content of an email.
type EmailContent struct {
	MessageID     string `json:"messageId"`
	FolderID      string `json:"folderId"`
	FromAddress   string `json:"fromAddress"`
	Sender        string `json:"sender"`
	ToAddress     string `json:"toAddress"`
	CcAddress     string `json:"ccAddress"`
	BccAddress    string `json:"bccAddress"`
	Subject       string `json:"subject"`
	Content       string `json:"content"`
	MailFormat    string `json:"mailFormat"`
	ReceivedTime  int64  `json:"receivedTime"`
	HasAttachment string `json:"hasAttachment"`
	Status2       string `json:"status2"`
}

// SendEmailRequest is the request to send an email.
type SendEmailRequest struct {
	ToAddress  string `json:"toAddress"`
	CcAddress  string `json:"ccAddress,omitempty"`
	BccAddress string `json:"bccAddress,omitempty"`
	Subject    string `json:"subject"`
	Content    string `json:"content"`
	MailFormat string `json:"mailFormat,omitempty"`
}

// SendEmailResponse is the response after sending an email.
type SendEmailResponse struct {
	Status struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
	} `json:"status"`
	Data struct {
		MessageID string `json:"messageId"`
		FolderID  string `json:"folderId"`
	} `json:"data"`
}

// ReplyRequest is the request to reply to an email.
type ReplyRequest struct {
	Content    string `json:"content"`
	MailFormat string `json:"mailFormat,omitempty"`
	ToAddress  string `json:"toAddress,omitempty"`
	CcAddress  string `json:"ccAddress,omitempty"`
}

// AttachmentInfo represents metadata about an email attachment.
type AttachmentInfo struct {
	AttachmentID   string `json:"attachmentId"`
	AttachmentName string `json:"attachmentName"`
	AttachmentSize int64  `json:"attachmentSize"`
	ContentType    string `json:"contentType"`
}

// AttachmentContent represents a downloaded attachment.
type AttachmentContent struct {
	AttachmentID string `json:"attachmentId"`
	ContentType  string `json:"contentType"`
	Data         string `json:"data"` // base64-encoded
}

// Calendar represents a calendar.
type Calendar struct {
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	IsDefault   bool   `json:"isdefault"`
}

// Event represents a calendar event.
type Event struct {
	UID         string    `json:"uid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       time.Time `json:"-"`
	End         time.Time `json:"-"`
	StartRaw    string    `json:"dateandtime"`
	EndRaw      string    `json:"enddate"`
	IsAllDay    bool      `json:"isallday"`
	CalendarUID string    `json:"calendaruid"`
}

// CreateEventRequest is the request to create a calendar event.
type CreateEventRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	Start       string `json:"start"` // RFC3339
	End         string `json:"end"`   // RFC3339
	IsAllDay    bool   `json:"isallday,omitempty"`
}

// UpdateEventRequest is the request to update a calendar event.
type UpdateEventRequest struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	Start       string `json:"start,omitempty"` // RFC3339
	End         string `json:"end,omitempty"`   // RFC3339
	IsAllDay    *bool  `json:"isallday,omitempty"`
}

// MailNotification represents a new mail notification from the poller.
type MailNotification struct {
	HasNew            bool   `json:"hasNew"`
	ZohoEmail         string `json:"zohoEmail,omitempty"`
	UnreadCount       int64  `json:"unreadCount,omitempty"`
	LatestMessageTime int64  `json:"latestMessageTime,omitempty"`
}

// ListEmailOpts contains options for listing emails.
type ListEmailOpts struct {
	FolderID  string
	Status    string
	Limit     int
	Start     int
	SortBy    string
	SortOrder string
}

// --- HTTP helpers ---

func (c *Client) doGet(path string) ([]byte, error) {
	return c.doRequest(http.MethodGet, path, nil)
}

func (c *Client) doPost(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPost, path, body)
}

func (c *Client) doDelete(path string) error {
	_, err := c.doRequest(http.MethodDelete, path, nil)
	return err
}

func (c *Client) doPut(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPut, path, body)
}

func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	apiURL := c.centralURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, apiURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.tunnelToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to Central: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Central returned %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// --- Mail operations ---

// GetFolders returns all mail folders.
func (c *Client) GetFolders() ([]Folder, error) {
	data, err := c.doGet("/api/v1/hq/mail/folders")
	if err != nil {
		return nil, err
	}
	var folders []Folder
	if err := json.Unmarshal(data, &folders); err != nil {
		return nil, fmt.Errorf("decode folders: %w", err)
	}
	return folders, nil
}

// SendEmail sends an email.
func (c *Client) SendEmail(req SendEmailRequest) (*SendEmailResponse, error) {
	data, err := c.doPost("/api/v1/hq/mail/send", req)
	if err != nil {
		return nil, err
	}
	var resp SendEmailResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode send response: %w", err)
	}
	return &resp, nil
}

// ListEmails lists emails with optional filtering.
func (c *Client) ListEmails(opts ListEmailOpts) ([]EmailSummary, error) {
	params := url.Values{}
	if opts.FolderID != "" {
		params.Set("folderId", opts.FolderID)
	}
	if opts.Status != "" {
		params.Set("status", opts.Status)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Start > 0 {
		params.Set("start", strconv.Itoa(opts.Start))
	}
	if opts.SortBy != "" {
		params.Set("sortBy", opts.SortBy)
	}
	if opts.SortOrder != "" {
		params.Set("sortOrder", opts.SortOrder)
	}

	path := "/api/v1/hq/mail/messages"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	data, err := c.doGet(path)
	if err != nil {
		return nil, err
	}
	var emails []EmailSummary
	if err := json.Unmarshal(data, &emails); err != nil {
		return nil, fmt.Errorf("decode emails: %w", err)
	}
	return emails, nil
}

// SearchEmails searches emails by query string.
func (c *Client) SearchEmails(query string, limit int) ([]EmailSummary, error) {
	params := url.Values{}
	params.Set("q", query)
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	data, err := c.doGet("/api/v1/hq/mail/search?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var emails []EmailSummary
	if err := json.Unmarshal(data, &emails); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}
	return emails, nil
}

// GetEmailContent returns the full content of an email.
func (c *Client) GetEmailContent(folderID, messageID string) (*EmailContent, error) {
	data, err := c.doGet(fmt.Sprintf("/api/v1/hq/mail/messages/%s/%s/content", folderID, messageID))
	if err != nil {
		return nil, err
	}
	var content EmailContent
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("decode email content: %w", err)
	}
	return &content, nil
}

// MarkAsRead marks messages as read.
func (c *Client) MarkAsRead(messageIDs []string) error {
	_, err := c.doPost("/api/v1/hq/mail/messages/mark-read", map[string][]string{
		"messageIds": messageIDs,
	})
	return err
}

// MarkAsUnread marks messages as unread.
func (c *Client) MarkAsUnread(messageIDs []string) error {
	_, err := c.doPost("/api/v1/hq/mail/messages/mark-unread", map[string][]string{
		"messageIds": messageIDs,
	})
	return err
}

// MoveEmail moves an email to a different folder.
func (c *Client) MoveEmail(messageID, destFolderID string) error {
	_, err := c.doPost(fmt.Sprintf("/api/v1/hq/mail/messages/%s/move", messageID), map[string]string{
		"destFolderId": destFolderID,
	})
	return err
}

// DeleteEmail deletes an email.
func (c *Client) DeleteEmail(folderID, messageID string) error {
	return c.doDelete(fmt.Sprintf("/api/v1/hq/mail/messages/%s/%s", folderID, messageID))
}

// ReplyToEmail replies to an existing email.
func (c *Client) ReplyToEmail(messageID string, req ReplyRequest) error {
	_, err := c.doPost(fmt.Sprintf("/api/v1/hq/mail/messages/%s/reply", messageID), req)
	return err
}

// GetAttachmentInfo returns metadata about email attachments.
func (c *Client) GetAttachmentInfo(folderID, messageID string) ([]AttachmentInfo, error) {
	data, err := c.doGet(fmt.Sprintf("/api/v1/hq/mail/messages/%s/%s/attachments", folderID, messageID))
	if err != nil {
		return nil, err
	}
	var info []AttachmentInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("decode attachment info: %w", err)
	}
	return info, nil
}

// GetAttachmentContent downloads an attachment (base64-encoded).
func (c *Client) GetAttachmentContent(folderID, messageID, attachmentID string) (*AttachmentContent, error) {
	data, err := c.doGet(fmt.Sprintf("/api/v1/hq/mail/messages/%s/%s/attachments/%s", folderID, messageID, attachmentID))
	if err != nil {
		return nil, err
	}
	var content AttachmentContent
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("decode attachment: %w", err)
	}
	return &content, nil
}

// GetNotifications checks for new mail notifications.
func (c *Client) GetNotifications() (*MailNotification, error) {
	data, err := c.doGet("/api/v1/hq/mail/notifications")
	if err != nil {
		return nil, err
	}
	var notif MailNotification
	if err := json.Unmarshal(data, &notif); err != nil {
		return nil, fmt.Errorf("decode notifications: %w", err)
	}
	return &notif, nil
}

// --- Calendar operations ---

// GetCalendars returns all calendars.
func (c *Client) GetCalendars() ([]Calendar, error) {
	data, err := c.doGet("/api/v1/hq/calendar/calendars")
	if err != nil {
		return nil, err
	}
	var calendars []Calendar
	if err := json.Unmarshal(data, &calendars); err != nil {
		return nil, fmt.Errorf("decode calendars: %w", err)
	}
	return calendars, nil
}

// GetCalendar returns a specific calendar.
func (c *Client) GetCalendar(calendarUID string) (*Calendar, error) {
	data, err := c.doGet(fmt.Sprintf("/api/v1/hq/calendar/calendars/%s", calendarUID))
	if err != nil {
		return nil, err
	}
	var cal Calendar
	if err := json.Unmarshal(data, &cal); err != nil {
		return nil, fmt.Errorf("decode calendar: %w", err)
	}
	return &cal, nil
}

// ListEvents returns events in a calendar within a date range.
func (c *Client) ListEvents(calendarUID, start, end string) ([]Event, error) {
	params := url.Values{}
	params.Set("start", start)
	params.Set("end", end)

	data, err := c.doGet(fmt.Sprintf("/api/v1/hq/calendar/calendars/%s/events?%s", calendarUID, params.Encode()))
	if err != nil {
		return nil, err
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}
	return events, nil
}

// CreateEvent creates a new calendar event.
func (c *Client) CreateEvent(calendarUID string, req CreateEventRequest) (*Event, error) {
	data, err := c.doPost(fmt.Sprintf("/api/v1/hq/calendar/calendars/%s/events", calendarUID), req)
	if err != nil {
		return nil, err
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("decode event: %w", err)
	}
	return &event, nil
}

// UpdateEvent updates an existing calendar event.
func (c *Client) UpdateEvent(calendarUID, eventUID string, req UpdateEventRequest) (*Event, error) {
	data, err := c.doPut(fmt.Sprintf("/api/v1/hq/calendar/calendars/%s/events/%s", calendarUID, eventUID), req)
	if err != nil {
		return nil, err
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("decode event: %w", err)
	}
	return &event, nil
}

// DeleteEvent deletes a calendar event.
func (c *Client) DeleteEvent(calendarUID, eventUID string) error {
	return c.doDelete(fmt.Sprintf("/api/v1/hq/calendar/calendars/%s/events/%s", calendarUID, eventUID))
}
