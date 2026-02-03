// Package session provides session lifecycle management for Poindexter
package session

// HatContract defines the pub/sub contract for a hat
type HatContract struct {
	Name       string   // Hat name
	Subscribes []string // Topics this hat listens to (triggers activation)
	Publishes  []string // Topics this hat can publish
}

// HatContracts defines the event contracts for all hats
var HatContracts = map[string]*HatContract{
	"explorer": {
		Name:       "explorer",
		Subscribes: []string{}, // Explorer is invoked manually or by system
		Publishes: []string{
			TopicPlanComplete,   // Can go directly to planning
			TopicDesignComplete, // Can go directly to design
			TopicTaskBlocked,    // Can report blockers
		},
	},
	"planner": {
		Name: "planner",
		Subscribes: []string{
			TopicTaskStarted, // First hat when task starts
		},
		Publishes: []string{
			TopicPlanComplete,   // Plan is ready
			TopicDesignComplete, // For simple tasks, can skip designer
			TopicTaskBlocked,    // Blocked during planning
		},
	},
	"designer": {
		Name: "designer",
		Subscribes: []string{
			TopicPlanComplete, // Triggered after planning
		},
		Publishes: []string{
			TopicDesignComplete, // Design is ready
			TopicTaskBlocked,    // Blocked during design
		},
	},
	"creator": {
		Name: "creator",
		Subscribes: []string{
			TopicPlanComplete,    // Can start directly from plan
			TopicDesignComplete,  // Start after design
			TopicReviewRejected,  // Revisions needed
			TopicResolved,        // Continue after blocker resolved
		},
		Publishes: []string{
			TopicImplementationDone, // Ready for review
			TopicTaskBlocked,        // Blocked during implementation
		},
	},
	"critic": {
		Name: "critic",
		Subscribes: []string{
			TopicImplementationDone, // Review implementation
		},
		Publishes: []string{
			TopicReviewApproved, // Implementation passes review
			TopicReviewRejected, // Needs fixes
			TopicTaskBlocked,    // Blocked during review
		},
	},
	"editor": {
		Name: "editor",
		Subscribes: []string{
			TopicReviewApproved, // Start after review passes
		},
		Publishes: []string{
			TopicTaskComplete, // Task finished (terminal)
			TopicTaskBlocked,  // Blocked during editing
		},
	},
	"resolver": {
		Name: "resolver",
		Subscribes: []string{
			TopicTaskBlocked, // Handle any blocker
		},
		Publishes: []string{
			TopicResolved,     // Blocker cleared, return to previous work
			TopicTaskComplete, // Sometimes resolver can complete task directly
		},
	},
}

// CanPublish checks if a hat is allowed to publish a topic
func CanPublish(hatName, topic string) bool {
	contract, ok := HatContracts[hatName]
	if !ok {
		return false
	}

	for _, t := range contract.Publishes {
		if t == topic {
			return true
		}
	}
	return false
}

// GetSubscribers returns the list of hats that subscribe to a topic
func GetSubscribers(topic string) []string {
	var subscribers []string
	for name, contract := range HatContracts {
		for _, t := range contract.Subscribes {
			if t == topic {
				subscribers = append(subscribers, name)
				break
			}
		}
	}
	return subscribers
}

// GetNextHatForTopic returns the primary hat that should handle a topic
// Uses priority: most specific subscriber wins
func GetNextHatForTopic(topic string) string {
	subscribers := GetSubscribers(topic)
	if len(subscribers) == 0 {
		return ""
	}

	// Priority order for ambiguous cases
	priority := map[string]int{
		"planner":  1,
		"designer": 2,
		"creator":  3,
		"critic":   4,
		"editor":   5,
		"resolver": 6,
		"explorer": 7,
	}

	// Return lowest priority number (highest priority)
	bestHat := subscribers[0]
	bestPriority := priority[bestHat]

	for _, hat := range subscribers[1:] {
		if p, ok := priority[hat]; ok && p < bestPriority {
			bestHat = hat
			bestPriority = p
		}
	}

	return bestHat
}

// GetContract returns the contract for a hat
func GetContract(hatName string) *HatContract {
	return HatContracts[hatName]
}
