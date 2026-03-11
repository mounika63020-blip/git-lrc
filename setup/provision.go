package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ProvisionLiveReviewUser calls ensure-cloud-user and creates an API key.
// Optional logf receives debug log messages.
func ProvisionLiveReviewUser(cbData *HexmosCallbackData, logf func(format string, args ...interface{})) (*SetupResult, error) {
	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(format, args...)
		}
	}

	reqBody := EnsureCloudUserRequest{
		Email:     cbData.Result.Data.Email,
		FirstName: cbData.Result.Data.FirstName,
		LastName:  cbData.Result.Data.LastName,
		Source:    "git-lrc",
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", CloudAPIURL+"/api/v1/auth/ensure-cloud-user", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cbData.Result.JWT)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact LiveReview API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ensure-cloud-user response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		log("ensure-cloud-user failed: status=%d body=%s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("ensure-cloud-user returned %d: %s", resp.StatusCode, string(respBody))
	}

	log("ensure-cloud-user: status=%d", resp.StatusCode)

	var ensureResp EnsureCloudUserResponse
	if err := json.Unmarshal(respBody, &ensureResp); err != nil {
		log("ensure-cloud-user parse error: %v  body=%s", err, string(respBody))
		return nil, fmt.Errorf("failed to parse ensure-cloud-user response: %w", err)
	}

	result := &SetupResult{
		Email:        ensureResp.Email,
		FirstName:    ensureResp.User.FirstName,
		LastName:     ensureResp.User.LastName,
		AvatarURL:    cbData.Result.Data.ProfilePicURL,
		UserID:       ensureResp.UserID.String(),
		OrgID:        ensureResp.OrgID.String(),
		AccessToken:  ensureResp.Tokens.AccessToken,
		RefreshToken: ensureResp.Tokens.RefreshToken,
	}

	if len(ensureResp.Organizations) > 0 {
		result.OrgName = ensureResp.Organizations[0].Name
		if result.OrgID == "" {
			result.OrgID = ensureResp.Organizations[0].ID.String()
		}
	}

	apiKeyReq := CreateAPIKeyRequest{Label: "LRC CLI Key"}
	apiKeyJSON, err := json.Marshal(apiKeyReq)
	if err != nil {
		return nil, err
	}

	apiKeyURL := fmt.Sprintf("%s/api/v1/orgs/%s/api-keys", CloudAPIURL, result.OrgID)
	log("creating API key: POST %s", apiKeyURL)
	req2, err := http.NewRequest("POST", apiKeyURL, bytes.NewReader(apiKeyJSON))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+result.AccessToken)

	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API key response: %w", err)
	}
	if resp2.StatusCode != http.StatusCreated && resp2.StatusCode != http.StatusOK {
		log("create API key failed: status=%d body=%s", resp2.StatusCode, string(respBody2))
		return nil, fmt.Errorf("create API key returned %d: %s", resp2.StatusCode, string(respBody2))
	}

	log("API key created: status=%d", resp2.StatusCode)

	var apiKeyResp CreateAPIKeyResponse
	if err := json.Unmarshal(respBody2, &apiKeyResp); err != nil {
		return nil, fmt.Errorf("failed to parse API key response: %w", err)
	}

	result.PlainAPIKey = apiKeyResp.PlainKey
	return result, nil
}
