package edupage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// FetchETestData returns the decoded e-test / homework card payload (materialData with cardsData etc.).
func (client *EdupageClient) FetchETestData(testID, superID string) (map[string]interface{}, error) {
	if client.Credentials.httpClient == nil {
		return nil, ErrorUnitialized
	}
	if testID == "" || superID == "" {
		return nil, errors.New("testid and superid are required")
	}

	payload := CreatePayload(map[string]string{"testid": testID, "superid": superID})
	resp, err := client.Credentials.httpClient.PostForm(
		"https://"+path.Join(client.Credentials.Server, "elearning", "?cmd=MaterialPlayer&akcia=getETestData"),
		payload,
	)
	if err != nil {
		return nil, fmt.Errorf("etest request failed: %w", err)
	}
	defer resp.Body.Close()

	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(response) < 5 {
		return nil, fmt.Errorf("etest request failed, bad response (%d bytes)", len(response))
	}

	// Response is "eqz:" + base64(json)
	body := bytes.TrimSpace(response[4:])
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("etest request failed, bad base64: %w", err)
	}

	var object map[string]interface{}
	if err := json.Unmarshal(bytes.Trim(decoded, "\x00"), &object); err != nil {
		return nil, fmt.Errorf("etest request failed, bad json: %w", err)
	}
	return object, nil
}

// FetchFile downloads a (possibly relative) EduPage resource using the logged-in session.
func (client *EdupageClient) FetchFile(src string) (*http.Response, error) {
	return client.FetchFileContext(context.Background(), src)
}

func allowedAttachmentURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "edupage.org" || strings.HasSuffix(host, ".edupage.org")
}

func (client *EdupageClient) FetchFileContext(ctx context.Context, src string) (*http.Response, error) {
	if client.Credentials.httpClient == nil {
		return nil, ErrorUnitialized
	}
	u := src
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	} else if !strings.HasPrefix(u, "http") {
		u = "https://" + client.Credentials.Server + "/" + strings.TrimPrefix(u, "/")
	}
	if !allowedAttachmentURL(u) {
		return nil, errors.New("only HTTPS edupage.org attachments are allowed")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// The session client refuses redirects; use a copy that follows them but keeps the cookie jar
	hc := *client.Credentials.httpClient
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 6 || !allowedAttachmentURL(req.URL.String()) {
			return errors.New("attachment redirect is not allowed")
		}
		return nil
	}
	return hc.Do(req)
}
