package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	cmdapp "github.com/HexmosTech/git-lrc/cmd"
	"github.com/HexmosTech/git-lrc/internal/naming"
	"github.com/HexmosTech/git-lrc/internal/reviewapi"
	"github.com/HexmosTech/git-lrc/internal/reviewdb"
	"github.com/HexmosTech/git-lrc/internal/reviewmodel"
	"github.com/HexmosTech/git-lrc/internal/reviewopts"
	"github.com/HexmosTech/git-lrc/internal/selfupdate"
	reviewpkg "github.com/HexmosTech/git-lrc/review"
	"github.com/urfave/cli/v2"
)

// Version information (set via ldflags during build)
const appVersion = "v0.1.43" // Semantic version - bump this for releases

var (
	version    = appVersion // Can be overridden via ldflags
	buildTime  = "unknown"
	gitCommit  = "unknown"
	reviewMode = "prod" // Set to "fake" by build-local-test via ldflags

	// Global review state for the web UI API
	currentReviewState *ReviewState
	reviewStateMu      sync.RWMutex
)

func isFakeReviewBuild() bool {
	return reviewpkg.IsFakeReviewBuild(reviewMode)
}

func fakeReviewWaitDuration() (time.Duration, error) {
	return reviewpkg.FakeReviewWaitDuration(os.Getenv("LRC_FAKE_REVIEW_WAIT"))
}

func buildFakeSubmitResponse() reviewmodel.DiffReviewCreateResponse {
	resp := reviewpkg.BuildFakeSubmitResponse(time.Now(), naming.GenerateFriendlyName())
	return reviewmodel.DiffReviewCreateResponse{
		ReviewID:     resp.ReviewID,
		Status:       resp.Status,
		FriendlyName: resp.FriendlyName,
	}
}

func buildFakeCompletedResult() *reviewmodel.DiffReviewResponse {
	resp := reviewpkg.BuildFakeCompletedResult()
	return &reviewmodel.DiffReviewResponse{
		Status:  resp.Status,
		Summary: resp.Summary,
		Files:   []reviewmodel.DiffReviewFileResult{},
	}
}

func pollReviewFake(reviewID string, pollInterval, wait time.Duration, verbose bool, cancel <-chan struct{}) (*reviewmodel.DiffReviewResponse, error) {
	if pollInterval <= 0 {
		pollInterval = 1 * time.Second
	}

	start := time.Now()
	deadline := start.Add(wait)
	fmt.Printf("Waiting for fake review completion (poll every %s, delay %s)...\r\n", pollInterval, wait)
	syncFileSafely(os.Stdout)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		now := time.Now()
		if !now.Before(deadline) {
			statusLine := fmt.Sprintf("Status: completed | elapsed: %s", now.Sub(start).Truncate(time.Second))
			fmt.Printf("\r%-80s\r\n", statusLine)
			syncFileSafely(os.Stdout)
			if verbose {
				log.Printf("fake review %s completed", reviewID)
			}
			return buildFakeCompletedResult(), nil
		}

		statusLine := fmt.Sprintf("Status: in_progress | elapsed: %s", now.Sub(start).Truncate(time.Second))
		fmt.Printf("\r%-80s", statusLine)
		syncFileSafely(os.Stdout)
		if verbose {
			log.Printf("fake review %s: %s", reviewID, statusLine)
		}

		select {
		case <-cancel:
			fmt.Printf("\r\n")
			syncFileSafely(os.Stdout)
			return nil, reviewapi.ErrPollCancelled
		case <-ticker.C:
		}
	}
}

const (
	commitMessageFile   = "livereview_commit_message"
	editorWrapperScript = "lrc_editor.sh"
	editorBackupFile    = ".lrc_editor_backup"
	pushRequestFile     = "livereview_push_request"
)

// highlightURL adds ANSI color to make served links stand out in terminals.
func highlightURL(url string) string {
	return "\033[36m" + url + "\033[0m"
}

func buildReviewURL(apiURL, reviewID string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(apiURL, "/"), "/api"), "/api/v1")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/#/reviews/%s", base, reviewID)
}

var baseFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "repo-name",
		Usage:   "repository name (defaults to current directory basename)",
		EnvVars: []string{"LRC_REPO_NAME"},
	},
	&cli.BoolFlag{
		Name:    "staged",
		Usage:   "use staged changes instead of working tree",
		EnvVars: []string{"LRC_STAGED"},
	},
	&cli.StringFlag{
		Name:    "range",
		Usage:   "git range for staged/working diff override (e.g., HEAD~1..HEAD)",
		EnvVars: []string{"LRC_RANGE"},
	},
	&cli.StringFlag{
		Name:    "commit",
		Usage:   "review a specific commit or commit range (e.g., HEAD, HEAD~1, HEAD~3..HEAD, abc123)",
		EnvVars: []string{"LRC_COMMIT"},
	},
	&cli.StringFlag{
		Name:    "diff-file",
		Usage:   "path to pre-generated diff file",
		EnvVars: []string{"LRC_DIFF_FILE"},
	},
	&cli.StringFlag{
		Name:    "api-url",
		Value:   reviewopts.DefaultAPIURL,
		Usage:   "LiveReview API base URL",
		EnvVars: []string{"LRC_API_URL"},
	},
	&cli.StringFlag{
		Name:    "api-key",
		Usage:   "API key for authentication (can be set in ~/.lrc.toml or env var)",
		EnvVars: []string{"LRC_API_KEY"},
	},
	&cli.StringFlag{
		Name:    "output",
		Value:   reviewopts.DefaultOutputFormat,
		Usage:   "output format: pretty or json",
		EnvVars: []string{"LRC_OUTPUT"},
	},
	&cli.StringFlag{
		Name:    "save-html",
		Usage:   "save formatted HTML output (GitHub-style review) to this file",
		EnvVars: []string{"LRC_SAVE_HTML"},
	},
	&cli.BoolFlag{
		Name:    "serve",
		Usage:   "start HTTP server to serve the HTML output (auto-creates HTML when omitted)",
		EnvVars: []string{"LRC_SERVE"},
	},
	&cli.IntFlag{
		Name:    "port",
		Usage:   "port for HTTP server (used with --serve)",
		Value:   8000,
		EnvVars: []string{"LRC_PORT"},
	},
	&cli.BoolFlag{
		Name:    "verbose",
		Usage:   "enable verbose output",
		EnvVars: []string{"LRC_VERBOSE"},
	},
	&cli.BoolFlag{
		Name:    "precommit",
		Usage:   "pre-commit mode: interactive prompts for commit decision (Ctrl-C=abort, Ctrl-S=skip+commit, Ctrl-V=vouch+commit, Enter=commit)",
		Value:   false,
		EnvVars: []string{"LRC_PRECOMMIT"},
	},
	&cli.BoolFlag{
		Name:    "skip",
		Usage:   "mark review as skipped and write attestation without contacting the API",
		EnvVars: []string{"LRC_SKIP"},
	},
	&cli.BoolFlag{
		Name:    "force",
		Usage:   "force rerun by removing existing attestation/hash for current tree",
		EnvVars: []string{"LRC_FORCE"},
	},
	&cli.BoolFlag{
		Name:    "vouch",
		Usage:   "vouch for changes manually without running AI review (records attestation with coverage stats from prior iterations)",
		EnvVars: []string{"LRC_VOUCH"},
	},
}

var debugFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "diff-source",
		Usage:   "diff source: working, staged, range, or file (debug override)",
		EnvVars: []string{"LRC_DIFF_SOURCE"},
		Hidden:  true,
	},
	&cli.DurationFlag{
		Name:    "poll-interval",
		Value:   reviewopts.DefaultPollInterval,
		Usage:   "interval between status polls",
		EnvVars: []string{"LRC_POLL_INTERVAL"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Value:   reviewopts.DefaultTimeout,
		Usage:   "maximum time to wait for review completion",
		EnvVars: []string{"LRC_TIMEOUT"},
	},
	&cli.StringFlag{
		Name:    "save-bundle",
		Usage:   "save the base64-encoded bundle to this file for inspection before sending",
		EnvVars: []string{"LRC_SAVE_BUNDLE"},
	},
	&cli.StringFlag{
		Name:    "save-json",
		Usage:   "save the JSON response to this file after completion",
		EnvVars: []string{"LRC_SAVE_JSON"},
	},
	&cli.StringFlag{
		Name:    "save-text",
		Usage:   "save formatted text output with comment markers to this file",
		EnvVars: []string{"LRC_SAVE_TEXT"},
	},
}

func main() {
	selfupdate.SetVersion(version)

	app := cmdapp.BuildApp(version, buildTime, gitCommit, baseFlags, debugFlags, cmdapp.Handlers{
		RunReviewSimple:       runReviewSimple,
		RunReviewDebug:        runReviewDebug,
		RunHooksInstall:       runHooksInstall,
		RunHooksUninstall:     runHooksUninstall,
		RunHooksEnable:        runHooksEnable,
		RunHooksDisable:       runHooksDisable,
		RunHooksStatus:        runHooksStatus,
		RunSelfUpdate:         selfupdate.RunSelfUpdate,
		RunReviewCleanup:      func(c *cli.Context) error { return reviewdb.RunReviewDBCleanup(c.Bool("verbose")) },
		RunAttestationTrailer: runAttestationTrailer,
		RunSetup:              runSetup,
		RunUI:                 runUI,
	})

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runReviewSimple(c *cli.Context) error {
	opts, err := reviewopts.BuildFromContext(c, false)
	if err != nil {
		return err
	}
	return runReviewWithOptions(opts)
}

func runReviewDebug(c *cli.Context) error {
	opts, err := reviewopts.BuildFromContext(c, true)
	if err != nil {
		return err
	}
	return runReviewWithOptions(opts)
}

// pickServePort tries the requested port, then increments by 1 up to maxTries to find a free port.
// It returns the listener itself (kept open) to avoid TOCTOU races where another
// process grabs the port between the check and the actual server start.
//
// On Windows, 0.0.0.0:<port> and 127.0.0.1:<port> are treated as separate bindings,
// so we must check both to detect if a port is truly occupied. On Linux/Mac,
// binding 0.0.0.0 already conflicts with 127.0.0.1, so a single check suffices.
func pickServePort(preferredPort, maxTries int) (net.Listener, int, error) {
	for i := 0; i < maxTries; i++ {
		candidate := preferredPort + i

		if runtime.GOOS == "windows" {
			// On Windows, check both addresses. If either is occupied, the port is busy.
			lnLocal, errLocal := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candidate))
			lnAll, errAll := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", candidate))

			if errLocal != nil || errAll != nil {
				// Port is busy on at least one address — close whichever succeeded
				if lnLocal != nil {
					lnLocal.Close()
				}
				if lnAll != nil {
					lnAll.Close()
				}
				continue
			}

			// Both succeeded — port is free. Close the 0.0.0.0 listener,
			// keep 127.0.0.1 (lrc only serves on localhost).
			lnAll.Close()
			return lnLocal, candidate, nil
		}

		// Linux/Mac: single bind on all interfaces is sufficient
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return ln, candidate, nil
		}
	}

	return nil, 0, fmt.Errorf("no available port found starting from %d", preferredPort)
}

// GIT HOOK MANAGEMENT
// =============================================================================

const (
	lrcMarkerBegin        = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd          = "# END lrc managed section"
	defaultGlobalHooksDir = ".git-hooks"
	hooksMetaFilename     = ".lrc-hooks-meta.json"
)

var managedHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"}
