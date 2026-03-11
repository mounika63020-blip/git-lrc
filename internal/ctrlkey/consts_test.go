package ctrlkey

import (
	"testing"

	"github.com/HexmosTech/git-lrc/internal/decisionflow"
)

func TestMapControlKeyToDecision(t *testing.T) {
	tests := []struct {
		name       string
		key        byte
		allowEnter bool
		wantCode   int
		wantOK     bool
	}{
		{name: "ctrl-c abort", key: ctrlCKey, allowEnter: false, wantCode: decisionflow.DecisionAbort, wantOK: true},
		{name: "ctrl-s skip", key: ctrlSKey, allowEnter: false, wantCode: decisionflow.DecisionSkip, wantOK: true},
		{name: "ctrl-v vouch", key: ctrlVKey, allowEnter: false, wantCode: decisionflow.DecisionVouch, wantOK: true},
		{name: "ctrl-y vouch", key: ctrlYKey, allowEnter: false, wantCode: decisionflow.DecisionVouch, wantOK: true},
		{name: "v fallback vouch", key: 'v', allowEnter: false, wantCode: decisionflow.DecisionVouch, wantOK: true},
		{name: "y fallback vouch", key: 'y', allowEnter: false, wantCode: decisionflow.DecisionVouch, wantOK: true},
		{name: "s fallback skip", key: 's', allowEnter: false, wantCode: decisionflow.DecisionSkip, wantOK: true},
		{name: "enter blocked when not allowed", key: '\n', allowEnter: false, wantCode: 0, wantOK: false},
		{name: "enter allowed when enabled", key: '\n', allowEnter: true, wantCode: decisionflow.DecisionCommit, wantOK: true},
		{name: "unknown key", key: 'x', allowEnter: false, wantCode: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCode, gotOK := mapControlKeyToDecision(tt.key, tt.allowEnter)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotCode != tt.wantCode {
				t.Fatalf("code = %d, want %d", gotCode, tt.wantCode)
			}
		})
	}
}
