package tools

// =============================================================================
// Mail & Calendar Tools - for AI sessions to interact with the user's email
// and calendar via Central's Zoho Mail proxy.
// =============================================================================

// MailListFoldersTool returns the tool definition for listing mail folders.
func MailListFoldersTool() Tool {
	return Tool{
		Name:        "mail_list_folders",
		Description: "List all email folders (Inbox, Sent, Drafts, etc.) with unread and total message counts.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		ReadOnly: true,
	}
}

// MailListMessagesTool returns the tool definition for listing emails.
func MailListMessagesTool() Tool {
	return Tool{
		Name:        "mail_list_messages",
		Description: "List emails in a folder. Returns subject, sender, date, and read status for each message.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"folder_id": map[string]any{
					"type":        "string",
					"description": "Folder ID to list messages from. Use mail_list_folders to discover folder IDs. Omit for Inbox.",
				},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"unread", "read", "flagged"},
					"description": "Filter by message status (default: all)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of messages to return (default: 20, max: 50)",
				},
			},
			"required": []string{},
		},
		ReadOnly: true,
	}
}

// MailSearchTool returns the tool definition for searching emails.
func MailSearchTool() Tool {
	return Tool{
		Name:        "mail_search",
		Description: "Search emails by keyword. Searches subject, sender, and body content.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query (keywords, sender email, etc.)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results (default: 20)",
				},
			},
			"required": []string{"query"},
		},
		ReadOnly: true,
	}
}

// MailReadTool returns the tool definition for reading an email.
func MailReadTool() Tool {
	return Tool{
		Name:        "mail_read",
		Description: "Read the full content of an email including headers, body, and attachment info.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"folder_id": map[string]any{
					"type":        "string",
					"description": "Folder ID the message is in",
				},
				"message_id": map[string]any{
					"type":        "string",
					"description": "Message ID to read",
				},
			},
			"required": []string{"folder_id", "message_id"},
		},
		ReadOnly: true,
	}
}

// MailSendTool returns the tool definition for sending an email.
func MailSendTool() Tool {
	return Tool{
		Name:        "mail_send",
		Description: "Send an email. The from address is automatically set to the user's Dex email.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Recipient email address(es), comma-separated",
				},
				"cc": map[string]any{
					"type":        "string",
					"description": "CC email address(es), comma-separated",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Email subject line",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Email body content (HTML supported)",
				},
			},
			"required": []string{"to", "subject", "body"},
		},
		ReadOnly: false,
	}
}

// MailReplyTool returns the tool definition for replying to an email.
func MailReplyTool() Tool {
	return Tool{
		Name:        "mail_reply",
		Description: "Reply to an existing email.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message_id": map[string]any{
					"type":        "string",
					"description": "Message ID to reply to",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Reply body content (HTML supported)",
				},
			},
			"required": []string{"message_id", "body"},
		},
		ReadOnly: false,
	}
}

// MailDeleteTool returns the tool definition for deleting an email.
func MailDeleteTool() Tool {
	return Tool{
		Name:        "mail_delete",
		Description: "Delete an email from a folder.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"folder_id": map[string]any{
					"type":        "string",
					"description": "Folder ID the message is in",
				},
				"message_id": map[string]any{
					"type":        "string",
					"description": "Message ID to delete",
				},
			},
			"required": []string{"folder_id", "message_id"},
		},
		ReadOnly: false,
	}
}

// CalendarListTool returns the tool definition for listing calendars.
func CalendarListTool() Tool {
	return Tool{
		Name:        "calendar_list",
		Description: "List all calendars available to the user.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		ReadOnly: true,
	}
}

// CalendarListEventsTool returns the tool definition for listing calendar events.
func CalendarListEventsTool() Tool {
	return Tool{
		Name:        "calendar_list_events",
		Description: "List events in a calendar within a date range.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_uid": map[string]any{
					"type":        "string",
					"description": "Calendar UID (use calendar_list to discover). Omit for default calendar.",
				},
				"start": map[string]any{
					"type":        "string",
					"description": "Start date/time in RFC3339 format (e.g., 2025-01-15T00:00:00Z)",
				},
				"end": map[string]any{
					"type":        "string",
					"description": "End date/time in RFC3339 format (e.g., 2025-01-22T00:00:00Z)",
				},
			},
			"required": []string{"start", "end"},
		},
		ReadOnly: true,
	}
}

// CalendarCreateEventTool returns the tool definition for creating a calendar event.
func CalendarCreateEventTool() Tool {
	return Tool{
		Name:        "calendar_create_event",
		Description: "Create a new calendar event.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_uid": map[string]any{
					"type":        "string",
					"description": "Calendar UID. Omit for default calendar.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Event title",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Event description",
				},
				"location": map[string]any{
					"type":        "string",
					"description": "Event location",
				},
				"start": map[string]any{
					"type":        "string",
					"description": "Start date/time in RFC3339 format",
				},
				"end": map[string]any{
					"type":        "string",
					"description": "End date/time in RFC3339 format",
				},
				"all_day": map[string]any{
					"type":        "boolean",
					"description": "Whether this is an all-day event",
				},
			},
			"required": []string{"title", "start", "end"},
		},
		ReadOnly: false,
	}
}

// CalendarUpdateEventTool returns the tool definition for updating a calendar event.
func CalendarUpdateEventTool() Tool {
	return Tool{
		Name:        "calendar_update_event",
		Description: "Update an existing calendar event.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_uid": map[string]any{
					"type":        "string",
					"description": "Calendar UID",
				},
				"event_uid": map[string]any{
					"type":        "string",
					"description": "Event UID to update",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "New title (omit to keep current)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "New description (omit to keep current)",
				},
				"start": map[string]any{
					"type":        "string",
					"description": "New start time in RFC3339 format (omit to keep current)",
				},
				"end": map[string]any{
					"type":        "string",
					"description": "New end time in RFC3339 format (omit to keep current)",
				},
			},
			"required": []string{"calendar_uid", "event_uid"},
		},
		ReadOnly: false,
	}
}

// CalendarDeleteEventTool returns the tool definition for deleting a calendar event.
func CalendarDeleteEventTool() Tool {
	return Tool{
		Name:        "calendar_delete_event",
		Description: "Delete a calendar event.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calendar_uid": map[string]any{
					"type":        "string",
					"description": "Calendar UID",
				},
				"event_uid": map[string]any{
					"type":        "string",
					"description": "Event UID to delete",
				},
			},
			"required": []string{"calendar_uid", "event_uid"},
		},
		ReadOnly: false,
	}
}
