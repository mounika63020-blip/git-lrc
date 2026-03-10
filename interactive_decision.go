package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

type decisionExecutionContext struct {
	precommit          bool
	verbose            bool
	initialMsg         string
	diffContent        []byte
	reviewID           string
	attestationWritten *bool
}

func normalizeDecisionCode(code int) int {
	return code
}

func precommitExitCodeForDecision(code int) int {
	code = normalizeDecisionCode(code)
	if code == decisionVouch {
		return decisionSkip
	}
	return code
}

func executeDecision(code int, message string, push bool, ctx decisionExecutionContext) error {
	code = normalizeDecisionCode(code)
	switch code {
	case decisionAbort:
		fmt.Println("\n❌ Commit aborted by user")
		return cli.Exit("", decisionAbort)
	case decisionCommit:
		finalMsg := strings.TrimSpace(message)
		if finalMsg == "" {
			finalMsg = strings.TrimSpace(ctx.initialMsg)
		}
		if ctx.precommit {
			return cli.Exit("", decisionCommit)
		}
		if err := runCommitAndMaybePush(finalMsg, push, ctx.verbose); err != nil {
			return err
		}
		return nil
	case decisionSkip:
		fmt.Println("\n⏭️  Review skipped, proceeding with commit")
		if err := ensureAttestation("skipped", ctx.verbose, ctx.attestationWritten); err != nil {
			return err
		}
		if ctx.precommit {
			return cli.Exit("", decisionSkip)
		}
		if err := runCommitAndMaybePush(strings.TrimSpace(message), push, ctx.verbose); err != nil {
			return err
		}
		return nil
	case decisionVouch:
		fmt.Println("\n✅ Vouched, proceeding with commit")
		if err := recordCoverageAndAttest("vouched", ctx.diffContent, ctx.reviewID, ctx.verbose, ctx.attestationWritten); err != nil {
			return fmt.Errorf("vouch failed: %w", err)
		}
		if ctx.precommit {
			return cli.Exit("", decisionSkip)
		}
		if err := runCommitAndMaybePush(strings.TrimSpace(message), push, ctx.verbose); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid decision code: %d", code)
	}
}
