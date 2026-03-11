package ctrlkey

import (
	"github.com/HexmosTech/git-lrc/interactive/keybinding"
	"github.com/HexmosTech/git-lrc/internal/decisionflow"
)

const (
	ctrlCKey byte = keybinding.CtrlCKey
	ctrlSKey byte = keybinding.CtrlSKey
	ctrlVKey byte = keybinding.CtrlVKey
	ctrlYKey byte = keybinding.CtrlYKey
)

func mapControlKeyToDecision(key byte, allowEnter bool) (int, bool) {
	code, ok := keybinding.MapControlKeyToDecision(key, allowEnter)
	if !ok {
		return 0, false
	}
	return mapKeybindingDecisionToDecisionFlow(code), true
}

func mapKeybindingDecisionToDecisionFlow(code int) int {
	switch code {
	case keybinding.DecisionAbort:
		return decisionflow.DecisionAbort
	case keybinding.DecisionSkip:
		return decisionflow.DecisionSkip
	case keybinding.DecisionVouch:
		return decisionflow.DecisionVouch
	default:
		return decisionflow.DecisionCommit
	}
}
