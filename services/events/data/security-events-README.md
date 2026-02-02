# Test Security Events Dataset - Documentation

## Overview

This dataset contains **113 realistic security events** spanning **January 1-3, 2026**, designed to test your unified security data platform and AI agent workflows.

## Dataset Statistics

- **Total Events:** 113
- **Time Range:** January 1, 2026 00:00:00 to January 3, 2026 23:59:59
- **Event Types:** 8 different security system types
- **Severity Levels:** INFO, LOW, MEDIUM, HIGH, CRITICAL
- **Correlated Scenarios:** 2 multi-event incident scenarios

## Event Type Distribution

| System | Count | Description |
|--------|-------|-------------|
| Video Surveillance | 29 | Motion detection, person/vehicle detection, weapon detection, loitering |
| Access Control | 28 | Granted/denied access, tailgating, forced entry, door held open |
| AI Threat Detection | 20 | Behavioral anomalies, safety violations, suspicious activity |
| Incident Management | 11 | Incident creation, updates, escalations |
| Intercom | 6 | Calls, emergency button presses |
| Acoustic Detection | 3 | Gunshot detection, breaking glass, alarms |
| Weapon Locker | 2 | Weapon checkout/return, unauthorized access |
| Radio Communications | 1 | Emergency alerts, transmissions |

## Severity Distribution

| Severity | Count | Percentage |
|----------|-------|------------|
| CRITICAL | 11 | 9.7% |
| HIGH | 16 | 14.2% |
| MEDIUM | 33 | 29.2% |
| LOW | 26 | 23.0% |
| INFO | 14 | 12.4% |

## Correlated Incident Scenarios

### Scenario 1: Armed Intruder (incident-2026-001)
**Date/Time:** January 2, 2026, 14:05:30  
**Location:** Building 5, Main Entrance  
**Event Count:** 8 correlated events  

**Timeline:**
1. **T-22s** - AI detects loitering (person waiting in lobby for 7 minutes)
2. **T-2s** - Access control: Badge denied (expired credentials)
3. **T-0.5s** - Acoustic system detects gunshot
4. **T+0s** - Video AI detects weapon (firearm, 94% confidence)
5. **T+2s** - Emergency intercom button pressed
6. **T+3s** - Officer radio emergency alert: "Code Red, Building 5"
7. **T+4s** - Weapon locker access: Officer checks out firearm for response
8. **T+5s** - Incident management system creates critical incident

**Use Case:** Tests agent's ability to:
- Correlate events across multiple systems
- Recognize escalating threat pattern
- Understand temporal relationships
- Synthesize coherent incident assessment
- Recommend appropriate Code Red response

### Scenario 2: Unauthorized Access After Hours (incident-2026-002)
**Date/Time:** January 3, 2026, 03:15:00  
**Location:** Building 3, Executive Wing (3rd Floor)  
**Event Count:** 5 correlated events  

**Timeline:**
1. **T-5min** - Access denied attempt #1 (insufficient access level)
2. **T-3min** - Access denied attempt #2 (same badge, different door)
3. **T-1min** - Access denied attempt #3 (same badge, third door)
4. **T+0s** - Door forced open (alarm triggered)
5. **T+10s** - Motion detected in restricted executive area

**Use Case:** Tests agent's ability to:
- Detect pattern of repeated access attempts
- Recognize reconnaissance behavior
- Correlate forced entry with previous denials
- Understand after-hours context increases severity
- Identify restricted area significance

## Event Schema Details

Each event follows the unified schema with these fields:

```json
{
  "event_id": "Unique identifier",
  "event_type": "category.subcategory.action",
  "source_system": "System that generated event",
  "timestamp": "ISO 8601 format with timezone",
  "location": {
    "site": "Campus identifier",
    "building": "Building identifier",
    "floor": "Floor number",
    "zone": "Specific zone/area",
    "camera_id/reader_id/etc": "Device identifier"
  },
  "severity": "CRITICAL|HIGH|MEDIUM|LOW|INFO",
  "entities": [
    {
      "entity_type": "person|vehicle|object|device",
      "entity_id": "Unique entity identifier",
      "attributes": {}
    }
  ],
  "payload": {
    // System-specific data
  },
  "metadata": {
    "ingestion_time": "When event entered system",
    "correlation_id": "Links related events (optional)",
    "tags": ["keywords", "for", "search"]
  }
}
```

## Interesting Events to Test

### High-Priority Events for Agent Testing

**Weapon Detections:**
- Search for `"event_type": "video.detection.weapon"` - Found in armed intruder scenario
- Test: Agent should immediately search runbooks, correlate events, create critical incident

**Forced Entry:**
- Search for `"event_type": "access.forced_entry"` - Found in unauthorized access scenario
- Test: Agent should correlate with previous access denials, recognize pattern

**Gunshot Detection:**
- Search for `"event_type": "acoustic.gunshot.detected"` - Found in armed intruder scenario
- Test: Agent should correlate with nearby weapon detection (same time/location)

**Emergency Intercom:**
- Search for `"event_type": "intercom.emergency.pressed"` - Found in armed intruder scenario
- Test: Agent should recognize panic button in context of other critical events

**Loitering in Executive Areas:**
- Search for `zone: "executive-wing"` + loitering events
- Test: Agent should assess higher threat level due to sensitive location

### Testing Correlation Logic

**Same Location, Same Time:**
```sql
-- Events within 30 seconds at Building 5, Floor 1
SELECT * FROM security_events
WHERE location->>'building' = 'building-5'
  AND location->>'floor' = '1'
  AND timestamp BETWEEN '2026-01-02T14:05:00Z' AND '2026-01-02T14:06:00Z'
ORDER BY timestamp;
```

**Same Person/Badge Across Events:**
```sql
-- Find all events involving badge-9999 (from armed intruder scenario)
SELECT * FROM security_events
WHERE entities @> '[{"entity_id": "badge-9999"}]'::jsonb
ORDER BY timestamp;
```

**Repeated Access Denials:**
```sql
-- Find patterns of repeated denials
SELECT 
  entities->0->>'entity_id' as badge,
  COUNT(*) as denial_count,
  MIN(timestamp) as first_attempt,
  MAX(timestamp) as last_attempt
FROM security_events
WHERE event_type = 'access.denied'
GROUP BY badge
HAVING COUNT(*) > 2
ORDER BY denial_count DESC;
```

## Agent Testing Scenarios

### Scenario 1: Critical Event Response
**Event:** Weapon detection at 2026-01-02T14:05:30Z  
**Expected Agent Behavior:**
1. Immediately classify as CRITICAL
2. Search runbooks for "weapon detection" procedures
3. Query database for events in Building 5 within 5 minutes
4. Discover 4+ correlated events (loitering, access denial, gunshot, intercom)
5. Synthesize: "Pattern indicates active threat - subject attempted entry, was denied, gunshot detected, weapon confirmed visually"
6. Recommend: Code Red, lock all Building 5 entrances, alert all officers, prepare law enforcement notification
7. Create incident with full correlation timeline
8. Send immediate alerts

### Scenario 2: Pattern Recognition
**Events:** Multiple access denials at 2026-01-03T03:10:00 - 03:15:00  
**Expected Agent Behavior:**
1. First denial: Classify as MEDIUM (after hours + executive area)
2. Second denial (2 min later): Recognize repeated attempt by same badge
3. Third denial (2 min later): Escalate severity - "Pattern of reconnaissance"
4. Forced entry: Correlate with previous denials - "Attempted entry failed 3x, now forced entry"
5. Motion detection: Confirm intrusion - "Subject now in restricted area"
6. Synthesize: "Unauthorized access scenario - reconnaissance followed by forced entry"
7. Recommend: Immediate security response, verify subject identity, check for theft/damage
8. Create HIGH severity incident with pattern analysis

### Scenario 3: False Positive Filtering
**Event:** Door held open in loading dock at 8:45 AM  
**Expected Agent Behavior:**
1. Check time: Business hours (8am-6pm)
2. Check location: Loading dock (expect deliveries)
3. Search for nearby events: Any scheduled deliveries? Any authorized personnel?
4. Assess: "Loading dock + business hours + no other suspicious activity = likely legitimate"
5. Classify: LOW severity
6. Action: Monitor but don't escalate
7. Log for pattern analysis (if happens daily at 8:45am, learn it's normal)

## PostgreSQL Import

To load into PostgreSQL:

```sql
-- Create table
CREATE TABLE security_events (
    event_id VARCHAR(100) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    source_system VARCHAR(50) NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    location JSONB NOT NULL,
    severity VARCHAR(20) NOT NULL,
    entities JSONB DEFAULT '[]',
    payload JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}'
);

-- Create indexes for common queries
CREATE INDEX idx_timestamp ON security_events(timestamp);
CREATE INDEX idx_event_type ON security_events(event_type);
CREATE INDEX idx_severity ON security_events(severity);
CREATE INDEX idx_location_building ON security_events((location->>'building'));
CREATE INDEX idx_correlation_id ON security_events((metadata->>'correlation_id'));

-- GIN indexes for JSONB queries
CREATE INDEX idx_location_gin ON security_events USING GIN(location);
CREATE INDEX idx_entities_gin ON security_events USING GIN(entities);

-- Load data (from Python)
import psycopg2
import json

conn = psycopg2.connect("postgresql://user:pass@host/db")
cur = conn.cursor()

with open('test-security-events.json', 'r') as f:
    events = json.load(f)

for event in events:
    cur.execute("""
        INSERT INTO security_events (
            event_id, event_type, source_system, timestamp, 
            location, severity, entities, payload, metadata
        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
    """, (
        event['event_id'],
        event['event_type'],
        event['source_system'],
        event['timestamp'],
        json.dumps(event['location']),
        event['severity'],
        json.dumps(event['entities']),
        json.dumps(event['payload']),
        json.dumps(event['metadata'])
    ))

conn.commit()
```

## Example Queries

### Find All Critical Events
```sql
SELECT event_id, event_type, timestamp, location->>'building' as building, payload
FROM security_events
WHERE severity = 'CRITICAL'
ORDER BY timestamp;
```

### Get Armed Intruder Scenario
```sql
SELECT event_id, event_type, timestamp, severity, location->>'zone' as zone
FROM security_events
WHERE metadata->>'correlation_id' = 'incident-2026-001'
ORDER BY timestamp;
```

### Find Events in Time Window
```sql
SELECT event_id, event_type, timestamp, location
FROM security_events
WHERE timestamp BETWEEN '2026-01-02T14:05:00Z' AND '2026-01-02T14:06:00Z'
  AND location->>'building' = 'building-5'
ORDER BY timestamp;
```

### Search by Location
```sql
SELECT event_id, event_type, timestamp, severity
FROM security_events
WHERE location->>'building' = 'building-3'
  AND location->>'zone' = 'executive-wing'
ORDER BY timestamp;
```

### Find Person Across Events
```sql
SELECT event_id, event_type, timestamp, location
FROM security_events
WHERE entities @> '[{"entity_id": "tracked-person-67890"}]'::jsonb
ORDER BY timestamp;
```

## Testing Recommendations

1. **Start with Correlated Scenarios:** Test your agent on the two incident scenarios first - they have clear patterns that should be detectable

2. **Test Individual Event Types:** Ensure agent can handle each event type correctly in isolation

3. **Test Correlation Logic:** Query for events in same location/time and verify agent correlates them

4. **Test Severity Assessment:** Verify agent correctly assesses severity based on context (time of day, location sensitivity, etc.)

5. **Test Runbook Search:** Ensure agent can find relevant procedures for different event types

6. **Test False Positive Filtering:** Verify agent doesn't over-react to routine events (door held open during business hours, etc.)

7. **Test Performance:** Measure how quickly agent processes events (target: <5 seconds per event)

## Data Quality Notes

- All timestamps are realistic and chronologically ordered
- Building/zone/device IDs are consistent across related events
- Person/badge IDs are reused to enable cross-event correlation
- Confidence scores and technical details are realistic
- S3 URIs follow realistic naming patterns
- Severity levels are contextually appropriate

This dataset should provide comprehensive coverage for testing your unified security data platform and AI agent workflows!
