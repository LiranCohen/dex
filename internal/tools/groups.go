package tools

import "slices"

// ToolGroup represents a semantic group of tools
type ToolGroup string

const (
	GroupFSRead   ToolGroup = "fs_read"   // File system read operations
	GroupFSWrite  ToolGroup = "fs_write"  // File system write operations
	GroupGitRead  ToolGroup = "git_read"  // Git read operations
	GroupGitWrite ToolGroup = "git_write" // Git write operations
	GroupGitHub   ToolGroup = "github"    // GitHub API operations
	GroupWeb      ToolGroup = "web"       // Web search/fetch
	GroupRuntime  ToolGroup = "runtime"   // Command execution
	GroupQuality  ToolGroup = "quality"   // Tests, lint, build
	GroupComplete ToolGroup = "complete"  // Task completion signals
	GroupMail     ToolGroup = "mail"      // Email operations
	GroupCalendar ToolGroup = "calendar"  // Calendar operations
)

// ToolGroups maps semantic groups to tool names
var ToolGroups = map[ToolGroup][]string{
	GroupFSRead: {
		"read_file",
		"list_files",
		"glob",
		"grep",
	},
	GroupFSWrite: {
		"write_file",
	},
	GroupGitRead: {
		"git_status",
		"git_diff",
		"git_log",
	},
	GroupGitWrite: {
		"git_init",
		"git_commit",
		"git_remote_add",
		"git_push",
	},
	GroupGitHub: {
		"github_create_repo",
		"github_create_pr",
	},
	GroupWeb: {
		"web_search",
		"web_fetch",
	},
	GroupRuntime: {
		"bash",
		"list_runtimes",
	},
	GroupQuality: {
		"run_tests",
		"run_lint",
		"run_build",
	},
	GroupComplete: {
		"task_complete",
	},
	GroupMail: {
		"mail_list_folders",
		"mail_list_messages",
		"mail_search",
		"mail_read",
		"mail_send",
		"mail_reply",
		"mail_delete",
	},
	GroupCalendar: {
		"calendar_list",
		"calendar_list_events",
		"calendar_create_event",
		"calendar_update_event",
		"calendar_delete_event",
	},
}

// ToolProfile defines a named set of tool capabilities
type ToolProfile string

const (
	ProfileExplorer ToolProfile = "explorer" // Read-only exploration
	ProfilePlanner  ToolProfile = "planner"  // Read-only planning
	ProfileCreator  ToolProfile = "creator"  // Full implementation access
	ProfileCritic   ToolProfile = "critic"   // Read-only review + quality
	ProfileEditor   ToolProfile = "editor"   // Full access including completion
)

// ProfilePolicy defines which tool groups are allowed/denied for a profile
type ProfilePolicy struct {
	Allow           []ToolGroup // Groups to include
	Deny            []string    // Specific tools to exclude (overrides Allow)
	RequireReadOnly bool        // Only include tools with ReadOnly=true
}

// ToolProfiles maps profiles to their policies
var ToolProfiles = map[ToolProfile]ProfilePolicy{
	ProfileExplorer: {
		Allow:           []ToolGroup{GroupFSRead, GroupGitRead, GroupWeb, GroupRuntime, GroupMail, GroupCalendar},
		Deny:            []string{"bash", "mail_send", "mail_reply", "mail_delete", "calendar_create_event", "calendar_update_event", "calendar_delete_event"}, // Read-only - no bash or write mail/calendar
		RequireReadOnly: true,
	},
	ProfilePlanner: {
		Allow:           []ToolGroup{GroupFSRead, GroupGitRead, GroupWeb, GroupRuntime, GroupMail, GroupCalendar},
		Deny:            []string{"bash", "mail_send", "mail_reply", "mail_delete", "calendar_create_event", "calendar_update_event", "calendar_delete_event"}, // Can read, not execute
		RequireReadOnly: true,
	},
	ProfileCreator: {
		Allow: []ToolGroup{GroupFSRead, GroupFSWrite, GroupGitRead, GroupGitWrite, GroupGitHub, GroupWeb, GroupRuntime, GroupQuality, GroupMail, GroupCalendar},
		// Full implementation access - no restrictions
	},
	ProfileCritic: {
		Allow:           []ToolGroup{GroupFSRead, GroupGitRead, GroupWeb, GroupQuality, GroupRuntime, GroupMail, GroupCalendar},
		Deny:            []string{"bash", "mail_send", "mail_reply", "mail_delete", "calendar_create_event", "calendar_update_event", "calendar_delete_event"}, // Review only
		RequireReadOnly: true,
	},
	ProfileEditor: {
		Allow: []ToolGroup{GroupFSRead, GroupFSWrite, GroupGitRead, GroupGitWrite, GroupGitHub, GroupWeb, GroupRuntime, GroupQuality, GroupComplete, GroupMail, GroupCalendar},
		// Full access including completion
	},
}

// HatProfiles maps hat names to tool profiles
var HatProfiles = map[string]ToolProfile{
	"explorer": ProfileExplorer, // Research only
	"planner":  ProfilePlanner,  // Can read, not write
	"designer": ProfilePlanner,  // Same as planner (architecture)
	"creator":  ProfileCreator,  // Full implementation access
	"critic":   ProfileCritic,   // Review only (read + quality gates)
	"editor":   ProfileEditor,   // Full access including completion
	"resolver": ProfileCreator,  // Needs full access to resolve blockers
}

// GetToolsForHat returns the tools available for a given hat
func GetToolsForHat(hat string) *Set {
	profile, exists := HatProfiles[hat]
	if !exists {
		// Safe default: explorer profile
		profile = ProfileExplorer
	}
	return ResolveProfileTools(profile)
}

// ResolveProfileTools builds a tool set from a profile's policy
func ResolveProfileTools(profile ToolProfile) *Set {
	policy, exists := ToolProfiles[profile]
	if !exists {
		return NewSet(nil) // Empty for unknown profile
	}

	// Collect tools from allowed groups
	allowed := make(map[string]bool)
	for _, group := range policy.Allow {
		for _, toolName := range ToolGroups[group] {
			allowed[toolName] = true
		}
	}

	// Remove denied tools
	for _, toolName := range policy.Deny {
		delete(allowed, toolName)
	}

	// Build tool list from registry
	var tools []Tool
	for toolName := range allowed {
		tool := GetToolByName(toolName)
		if tool == nil {
			continue
		}

		// Check annotation requirements
		if policy.RequireReadOnly && !tool.ReadOnly {
			continue
		}

		tools = append(tools, *tool)
	}

	return NewSet(tools)
}

// GetProfileForHat returns the tool profile for a hat
func GetProfileForHat(hat string) ToolProfile {
	if profile, exists := HatProfiles[hat]; exists {
		return profile
	}
	return ProfileExplorer // Safe default
}

// GetToolGroupsForProfile returns the groups included in a profile
func GetToolGroupsForProfile(profile ToolProfile) []ToolGroup {
	if policy, exists := ToolProfiles[profile]; exists {
		return policy.Allow
	}
	return nil
}

// IsToolAllowedForHat checks if a specific tool is allowed for a hat
func IsToolAllowedForHat(toolName, hat string) bool {
	toolSet := GetToolsForHat(hat)
	return toolSet.Has(toolName)
}

// GetAllToolGroups returns all defined tool groups
func GetAllToolGroups() []ToolGroup {
	return []ToolGroup{
		GroupFSRead,
		GroupFSWrite,
		GroupGitRead,
		GroupGitWrite,
		GroupGitHub,
		GroupWeb,
		GroupRuntime,
		GroupQuality,
		GroupComplete,
		GroupMail,
		GroupCalendar,
	}
}

// GetToolsInGroup returns all tool names in a group
func GetToolsInGroup(group ToolGroup) []string {
	if tools, exists := ToolGroups[group]; exists {
		return slices.Clone(tools)
	}
	return nil
}
