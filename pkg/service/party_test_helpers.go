package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func partySnapshotMessage(t *testing.T, requester string) string {
	t.Helper()
	payload := map[string]any{
		"sessionID":       "party",
		"namespace":       "ns",
		"requesterUserID": requester,
		"payload": map[string]any{
			"Members": []map[string]any{
				{"ID": requester, "Status": "JOINED"},
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return string(data)
}

func partyMembersSnapshotMessage(t *testing.T, partyID string, members ...string) string {
	t.Helper()
	if partyID == "" {
		partyID = "party"
	}
	payload := map[string]any{
		"payload": map[string]any{
			"ID":      partyID,
			"Members": []map[string]any{},
		},
	}
	memberList := payload["payload"].(map[string]any)["Members"].([]map[string]any)
	for _, member := range members {
		if strings.TrimSpace(member) == "" {
			continue
		}
		memberList = append(memberList, map[string]any{
			"ID":       member,
			"Status":   "JOINED",
			"StatusV2": "JOINED",
		})
	}
	payload["payload"].(map[string]any)["Members"] = memberList
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal members snapshot: %v", err)
	}
	return string(data)
}
