package attestation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

// ComputePriorCoverage calculates how many lines in the current diff were
// already AI-reviewed in prior iterations for the same branch.
func ComputePriorCoverage(db *sql.DB, branch, currentTreeHash string, currentFiles []FileEntry) (CoverageResult, error) {
	result := CoverageResult{}

	totalIter, err := CountIterations(db, branch)
	if err != nil {
		return result, err
	}
	result.Iterations = totalIter + 1

	priorSessions, err := GetPriorReviewedSessions(db, branch)
	if err != nil {
		return result, err
	}
	result.PriorReviewCount = len(priorSessions)

	if len(priorSessions) == 0 || len(currentFiles) == 0 {
		result.TotalLines = CountTotalNewLines(currentFiles)
		return result, nil
	}

	result.TotalLines = CountTotalNewLines(currentFiles)
	if result.TotalLines == 0 {
		return result, nil
	}

	coveredLines := make(map[string]bool)

	for _, session := range priorSessions {
		if session.TreeHash == currentTreeHash {
			for _, f := range currentFiles {
				MarkAllNewLines(coveredLines, f)
			}
			continue
		}

		changedFiles, err := DiffTreeFiles(session.TreeHash, currentTreeHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping review session %d: could not diff trees %s..%s: %v\n", session.ID, session.TreeHash, currentTreeHash, err)
			continue
		}

		changedFileSet := make(map[string]bool)
		for _, cf := range changedFiles {
			changedFileSet[cf] = true
		}

		var priorFiles []FileEntry
		if session.DiffFiles != "" {
			if umErr := json.Unmarshal([]byte(session.DiffFiles), &priorFiles); umErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: malformed diff_files in review session %d: %v\n", session.ID, umErr)
				continue
			}
		}

		priorFileMap := make(map[string][]HunkRange)
		for _, pf := range priorFiles {
			priorFileMap[pf.FilePath] = pf.Hunks
		}

		for _, cf := range currentFiles {
			if !changedFileSet[cf.FilePath] {
				MarkAllNewLines(coveredLines, cf)
			} else if priorHunks, ok := priorFileMap[cf.FilePath]; ok {
				MarkOverlappingLines(coveredLines, cf.FilePath, cf.Hunks, priorHunks, session.TreeHash, currentTreeHash)
			}
		}
	}

	result.CoveredLines = len(coveredLines)
	if result.TotalLines > 0 {
		result.PriorAICovPct = float64(result.CoveredLines) / float64(result.TotalLines) * 100
	}

	return result, nil
}
