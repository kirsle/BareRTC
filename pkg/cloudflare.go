package barertc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
)

// GetCloudflareTURNCredentials makes an API call to Cloudflare to get TURN server
// credentials using their hosted service.
//
// This is used only if your settings.toml has a Cloudflare TURN Key ID and API Key
// configured. When clients request WebRTC configuration, it will get temporary
// credentials from Cloudflare.
//
// Documentation: https://developers.cloudflare.com/realtime/turn/
//
// This returns the JSON string from their generate-ice-servers endpoint, or an
// error if something went wrong.
func GetCloudflareTURNCredentials() (string, error) {

	if config.Current.TURN.CloudflareTurnKeyID == "" || config.Current.TURN.CloudflareTurnKeyAPIToken == "" {
		return "", errors.New("cloudflare API keys not configured for TURN server")
	}

	var endpoint = fmt.Sprintf(
		"https://rtc.live.cloudflare.com/v1/turn/keys/%s/credentials/generate-ice-servers",
		config.Current.TURN.CloudflareTurnKeyID,
	)

	// JSON request body.
	jsonStr, err := json.Marshal(
		struct {
			TTL int `json:"ttl"`
		}{
			TTL: 86400,
		},
	)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonStr))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+config.Current.TURN.CloudflareTurnKeyAPIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}
