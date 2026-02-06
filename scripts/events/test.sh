#!/bin/bash

  BASEURL="http://localhost:8081"
  APPNAME="root_agent"
  USER="testuser"
  SESSION="testsession"

  SESSION_ENDPOINT="${BASEURL}/api/apps/${APPNAME}/users/${USER}/sessions/${SESSION}"

  # Create session
  echo "Creating session..."
  curl -X POST "$SESSION_ENDPOINT"
  echo -e "\n"

  # Endpoint for invoking the root agent
  ENDPOINT="${BASEURL}/api/run"

  # Sample query using stateDelta (recommended)
  QUERY=$(cat <<'EOF'
{
    "appName": "root_agent",
    "userId": "testuser",
    "sessionId": "testsession",
    "newMessage": {
        "role": "user",
        "parts": [{
            "text": "Analyze this camera offline event"
        }]
    },
    "stateDelta": {
        "event_data": "{\"event_id\":\"evt-camera-20260103-230906-6442\",\"event_type\":\"video.system.camera_offline\",\"source_system\":\"vms-milestone\",\"timestamp\":\"2026-01-03T23:09:06Z\",\"location\":{\"site\":\"campus-north\",\"building\":\"building-1\",\"floor\":\"2\",\"zone\":\"hallway-south\",\"camera_id\":\"cam-5-cafe-04\"},\"severity\":\"MEDIUM\",\"entities\":[],\"payload\":{\"camera_id\":\"cam-5-cafe-04\",\"frame_number\":54964},\"metadata\":{\"ingestion_time\":\"2026-01-03T23:09:06.113000Z\",\"tags\":[]}}"
    }
}
EOF
  )

  # Make the curl call
  echo "Sending event to agent..."
  curl -X POST \
       -H "Content-Type: application/json" \
       -d "$QUERY" \
       "$ENDPOINT" | jq .