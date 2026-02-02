package models

import "time"

type SecurityEvent struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	SourceSystem string    `json:"source_system"`
	Timestamp    time.Time `json:"timestamp"`
	Location     Location  `json:"location"`
	Severity     string    `json:"severity"`
	Entities     []Entity  `json:"entities"`
	Payload      any       `json:"payload"`
	Metadata     Metadata  `json:"metadata"`
}

type Location struct {
	Site        string      `json:"site"`
	Building    string      `json:"building"`
	Floor       string      `json:"floor"`
	Zone        string      `json:"zone"`
	Coordinates Coordinates `json:"coordinates"`
}

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Entity struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Attributes any    `json:"attributes"`
}

type Metadata struct {
	IngestionTime time.Time `json:"ingestion_time"`
	CorrelationID *string   `json:"correlation_id,omitempty"`
	Tags          []string  `json:"tags"`
}
