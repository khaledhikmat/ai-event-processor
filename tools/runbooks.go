package tools

import (
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// **** Models
// RunbooksArgs defines the arguments for the search_runbooks tool.
type RunbooksArgs struct {
	EventType string `json:"event_type" description:"Type of event for runbooks tool"`
	Query     string `json:"query" description:"Search query for runbooks tool"`
}

// RunbooksResult defines the response structure for the search_runbooks tool.
type RunbooksResult struct {
	RelevantProcedures    []RelevantProcedure `json:"relevant_procedures"`
	ApplicableGuidance    string              `json:"applicable_guidance"`
	SpecialConsiderations []string            `json:"special_considerations"`
	ConflictingGuidance   *string             `json:"conflicting_guidance"`
	Confidence            float64             `json:"confidence"`
	Gaps                  []string            `json:"gaps"`
}

type RelevantProcedure struct {
	DocumentTitle  string   `json:"document_title"`
	Section        string   `json:"section"`
	Guidance       string   `json:"guidance"`
	RelevanceScore float64  `json:"relevance_score"`
	DecisionPoints []string `json:"decision_points"`
}

// **** Constructor
// NewRunbooksTool creates a new ADK tool for retrieving runbooks.
func NewRunbooksTool() (tool.Tool, *RunbooksProvider, error) {
	wp := &RunbooksProvider{}
	t, err := functiontool.New(functiontool.Config{
		Name:        "search_runbooks",
		Description: "Search for possible runbooks outcome (based on event type category) based on a search query.",
	}, wp.SearchRunbooks)
	return t, wp, err
}

// **** Methods
// RunbooksProvider implements the search_runbooks tool.
type RunbooksProvider struct {
}

// Close cleans up any resources.
func (wp *RunbooksProvider) Close() error {
	return nil
}

func (wp *RunbooksProvider) SearchRunbooks(_ tool.Context, args RunbooksArgs) (RunbooksResult, error) {
	aggregatedEventType := wp.toEventType(args.EventType)

	// Provide hardcoded responses for specific event types
	switch aggregatedEventType {
	case "video_detection":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Weapon Detection Response Protocol (WD-001)",
					Section:        "Initial Response",
					Guidance:       "When weapon detected: 1) Verify via video review, 2) If business hours + public area = Code Yellow, 3) If after hours OR restricted area = Code Red...",
					RelevanceScore: 0.95,
					DecisionPoints: []string{
						"Time of day: business hours (8am-6pm) vs after hours",
						"Location type: public area vs restricted area",
						"Threat level: weapon type (firearm vs knife vs tool)",
					},
				},
			},
			ApplicableGuidance: "Based on WD-001: Business hours (2:05 PM) + public area (main lobby) indicates Code Yellow response. However, section 3.2 notes that confirmed firearms always require Code Red regardless of time/location.",
			SpecialConsiderations: []string{
				"Construction areas: Verify if tools misidentified as weapons",
				"Training facility: Check if authorized training session in progress",
				"Security personnel: Confirm if subject is authorized security officer",
			},
			ConflictingGuidance: nil,
			Confidence:          0.90,
			Gaps:                []string{},
		}, nil
	case "access_control_violation":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Access Control Violation Response (AC-003)",
					Section:        "Initial Response",
					Guidance:       "When denial: 1) Single denial: Log and monitor (no immediate action unless repeated) 2) Multiple denials (3+ in 30 min): Investigate for badge theft or probing behavior 3) After hours: Escalate to security officer for verification 4) Sensitive areas: Immediate notification regardless of time",
					RelevanceScore: 0.95,
					DecisionPoints: []string{
						"Time of day: business hours (8am-6pm) vs after hours",
						"Location type: Home Office or Regional Office vs Data Center or R&D Lab",
						"Repeat attempts: single vs multiple (3+ in 30 min)",
					},
				},
			},
			ApplicableGuidance: "Based on AC-003: Single access denial during business hours in non-sensitive area requires logging only. Multiple attempts or after-hours access requires escalation.",
			SpecialConsiderations: []string{
				"Badge issues: Check if employee badge needs replacement",
				"New employees: Verify if access permissions are properly configured",
				"Sensitive areas: Data centers and R&D labs require immediate notification",
			},
			ConflictingGuidance: nil,
			Confidence:          0.90,
			Gaps:                []string{},
		}, nil
	case "acoustic_violation":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Acoustic Anomaly Response (AA-005)",
					Section:        "Sound Detection Protocol",
					Guidance:       "When acoustic anomaly detected: 1) Classify sound type (gunshot, glass break, alarm, shouting) 2) Correlate with video surveillance 3) Gunshot or glass break = immediate Code Red 4) Shouting or disturbance = Code Yellow with officer dispatch",
					RelevanceScore: 0.92,
					DecisionPoints: []string{
						"Sound classification: gunshot vs glass break vs shouting vs alarm",
						"Location: public vs restricted area",
						"Video correlation: confirmed visual vs audio only",
					},
				},
			},
			ApplicableGuidance: "Based on AA-005: Acoustic anomalies require immediate video correlation. Gunshot sounds always trigger Code Red. Other sounds require classification before response escalation.",
			SpecialConsiderations: []string{
				"Construction zones: May generate false positives for glass break or impact sounds",
				"Testing areas: Check for scheduled audio system testing",
				"Environmental factors: Thunder, vehicles, or machinery may trigger false alarms",
			},
			ConflictingGuidance: nil,
			Confidence:          0.85,
			Gaps:                []string{},
		}, nil
	case "monitoring":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Video System Monitoring (VS-010)",
					Section:        "System Health & Alerts",
					Guidance:       "For system alerts: 1) Camera offline: Log and notify maintenance within 4 hours 2) Recording failure: Immediate escalation to IT 3) Network issues: Check connectivity and failover systems 4) Storage capacity: Alert when 80% full, critical at 90%",
					RelevanceScore: 0.88,
					DecisionPoints: []string{
						"Alert type: camera offline vs recording failure vs network issue",
						"Location criticality: high-security area vs general monitoring",
						"Duration: intermittent vs sustained outage",
					},
				},
			},
			ApplicableGuidance: "Based on VS-010: System health issues should be logged and addressed based on location criticality. High-security areas require immediate attention, general areas within 4 hours.",
			SpecialConsiderations: []string{
				"Scheduled maintenance: Verify if outage is planned",
				"Weather conditions: Check for environmental impact on outdoor cameras",
				"Redundancy: Ensure backup recording systems are operational",
			},
			ConflictingGuidance: nil,
			Confidence:          0.87,
			Gaps:                []string{},
		}, nil
	case "incident_mgmt":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Incident Management Protocol (IM-001)",
					Section:        "Incident Response Workflow",
					Guidance:       "For incident management: 1) Create incident record with timestamp and location 2) Assign severity level (Critical/High/Medium/Low) 3) Dispatch appropriate personnel 4) Establish communication chain 5) Document all actions and outcomes 6) Conduct post-incident review",
					RelevanceScore: 0.93,
					DecisionPoints: []string{
						"Severity assessment: Critical vs High vs Medium vs Low",
						"Resource allocation: security vs medical vs fire vs law enforcement",
						"Communication: internal only vs external notification required",
					},
				},
			},
			ApplicableGuidance: "Based on IM-001: All incidents require proper documentation and severity assessment. Critical incidents require immediate external notification (police/fire/medical). Lower severity handled internally with post-incident review.",
			SpecialConsiderations: []string{
				"Legal requirements: Some incidents require mandatory reporting to authorities",
				"Media relations: High-profile incidents may require PR coordination",
				"Insurance: Document thoroughly for potential claims",
			},
			ConflictingGuidance: nil,
			Confidence:          0.91,
			Gaps:                []string{},
		}, nil
	case "intercom_communication":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Intercom Communication Protocol (IC-007)",
					Section:        "Two-Way Communication Guidelines",
					Guidance:       "For intercom communications: 1) Emergency calls: Immediate response required, escalate to security officer 2) Access requests: Verify identity before granting access 3) Information requests: Provide standard information only 4) Suspicious behavior: Document and alert security team",
					RelevanceScore: 0.89,
					DecisionPoints: []string{
						"Call type: emergency vs access request vs information",
						"Caller verification: identified vs unknown",
						"Time of day: business hours vs after hours",
					},
				},
			},
			ApplicableGuidance: "Based on IC-007: All intercom communications should be logged. Emergency calls require immediate response. Access requests must verify caller identity before granting entry.",
			SpecialConsiderations: []string{
				"Language barriers: Have translation services available",
				"Hearing impaired: Ensure visual alternatives available",
				"Recording: All intercom communications are recorded for security purposes",
			},
			ConflictingGuidance: nil,
			Confidence:          0.86,
			Gaps:                []string{},
		}, nil
	case "radio_communication":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Radio Communication Standards (RC-008)",
					Section:        "Radio Protocol & Codes",
					Guidance:       "For radio communications: 1) Use standard 10-codes for efficiency 2) Code Red: Emergency/weapons 3) Code Yellow: Suspicious activity 4) Code Blue: Medical emergency 5) Code Green: All clear 6) Maintain radio discipline and clear communication",
					RelevanceScore: 0.90,
					DecisionPoints: []string{
						"Code level: Red vs Yellow vs Blue vs Green",
						"Priority: emergency vs routine communication",
						"Audience: all units vs specific unit",
					},
				},
			},
			ApplicableGuidance: "Based on RC-008: Radio communications must be clear, concise, and follow standard codes. Emergency codes (Red, Blue) take priority over all other traffic. All transmissions are logged.",
			SpecialConsiderations: []string{
				"Channel selection: Use appropriate channel for message sensitivity",
				"Battery management: Ensure radios are charged and functional",
				"Backup systems: Cellular fallback if radio system fails",
			},
			ConflictingGuidance: nil,
			Confidence:          0.88,
			Gaps:                []string{},
		}, nil
	case "locker_management":
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "Locker Management Protocol (LM-012)",
					Section:        "Access & Security",
					Guidance:       "For locker management: 1) Assignment: Link locker to employee badge 2) Access violations: Log and investigate repeated failures 3) Abandoned lockers: Clear after 30 days notice 4) Maintenance: Inspect quarterly for damage or tampering",
					RelevanceScore: 0.84,
					DecisionPoints: []string{
						"Event type: normal access vs violation vs maintenance",
						"User status: active employee vs terminated vs visitor",
						"Location: general vs high-security area",
					},
				},
			},
			ApplicableGuidance: "Based on LM-012: Locker access is tied to employee credentials. Violations should be logged and patterns investigated. Abandoned lockers cleared after appropriate notice period.",
			SpecialConsiderations: []string{
				"Privacy: Locker searches require supervisor approval and documentation",
				"Contraband: If prohibited items found, escalate to security and HR",
				"Emergency access: Master keys available for emergency situations",
			},
			ConflictingGuidance: nil,
			Confidence:          0.82,
			Gaps:                []string{},
		}, nil
	default:
		return RunbooksResult{
			RelevantProcedures: []RelevantProcedure{
				{
					DocumentTitle:  "General Security Response (GS-099)",
					Section:        "Unknown Event Handling",
					Guidance:       "For unclassified events: 1) Document all available information 2) Assess potential risk level 3) Contact security supervisor for guidance 4) Monitor situation until classified 5) Update procedures if new event type identified",
					RelevanceScore: 0.70,
					DecisionPoints: []string{
						"Risk assessment: potential threat vs benign",
						"Information available: complete vs partial",
						"Urgency: immediate attention vs routine follow-up",
					},
				},
			},
			ApplicableGuidance: "Based on GS-099: Unknown or unclassified events require documentation and supervisor consultation. Default to caution and escalate if any indicators suggest potential security concern.",
			SpecialConsiderations: []string{
				"New event types: May require procedure updates",
				"System issues: Could indicate sensor or integration problems",
				"Documentation: Thorough recording helps identify patterns",
			},
			ConflictingGuidance: nil,
			Confidence:          0.65,
			Gaps: []string{
				"Event type not in current runbook database",
				"May require custom response procedure",
			},
		}, nil
	}
}

func (wp *RunbooksProvider) toEventType(eventType string) string {
	if strings.HasPrefix(eventType, "video.detection.") {
		return "video_detection"
	} else if strings.HasPrefix(eventType, "video.analytics.") {
		return "video_detection"
	} else if strings.HasPrefix(eventType, "video.system.") {
		return "monitoring"
	} else if strings.HasPrefix(eventType, "access.") {
		return "access_control_violation"
	} else if strings.HasPrefix(eventType, "acoustic.") {
		return "acoustic_violation"
	} else if strings.HasPrefix(eventType, "ai.") {
		return "video_detection"
	} else if strings.HasPrefix(eventType, "incident.") {
		return "incident_mgmt"
	} else if strings.HasPrefix(eventType, "intercom.") {
		return "intercom_communication"
	} else if strings.HasPrefix(eventType, "radio.") {
		return "radio_communication"
	} else if strings.HasPrefix(eventType, "locker.") {
		return "locker_management"
	} else {
		return "other"
	}
}
