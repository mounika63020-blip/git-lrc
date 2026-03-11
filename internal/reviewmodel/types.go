package reviewmodel

import "fmt"

// DiffReviewRequest models the POST payload to /api/v1/diff-review.
type DiffReviewRequest struct {
	DiffZipBase64 string `json:"diff_zip_base64"`
	RepoName      string `json:"repo_name"`
}

// DiffReviewResponse models the response from GET /api/v1/diff-review/:id.
type DiffReviewResponse struct {
	Status       string                 `json:"status"`
	Summary      string                 `json:"summary,omitempty"`
	Files        []DiffReviewFileResult `json:"files,omitempty"`
	Message      string                 `json:"message,omitempty"`
	FriendlyName string                 `json:"friendly_name,omitempty"`
}

type DiffReviewCreateResponse struct {
	ReviewID     string `json:"review_id"`
	Status       string `json:"status"`
	FriendlyName string `json:"friendly_name,omitempty"`
	UserEmail    string `json:"user_email,omitempty"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

type DiffReviewFileResult struct {
	FilePath string              `json:"file_path"`
	Hunks    []DiffReviewHunk    `json:"hunks"`
	Comments []DiffReviewComment `json:"comments"`
}

type DiffReviewHunk struct {
	OldStartLine int    `json:"old_start_line"`
	OldLineCount int    `json:"old_line_count"`
	NewStartLine int    `json:"new_start_line"`
	NewLineCount int    `json:"new_line_count"`
	Content      string `json:"content"`
}

type DiffReviewComment struct {
	Line     int    `json:"line"`
	Content  string `json:"content"`
	Severity string `json:"severity"`
	Category string `json:"category"`
}
