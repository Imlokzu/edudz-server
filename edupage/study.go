package edupage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// LessonPlan returns the authenticated student's published day-plan and curriculum.
// The dashboard/gcall protocol is also documented by EdupageAPI/edupage-api.
func (client *EdupageClient) LessonPlan(ctx context.Context, date time.Time) ([]map[string]interface{}, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://"+client.Credentials.Server+"/dashboard/eb.php?mode=ttday", nil)
	resp, err := client.Credentials.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	gpid := regexp.MustCompile(`gpid=([0-9]+)`).FindSubmatch(body)
	gsh := regexp.MustCompile(`gsh=([^"&\s]+)`).FindSubmatch(body)
	if len(gpid) < 2 || len(gsh) < 2 {
		return nil, errors.New("lesson plan is unavailable")
	}
	id, _ := strconv.Atoi(string(gpid[1]))
	user, err := client.GetUser(false)
	if err != nil {
		return nil, err
	}
	loggedUser := user.UserID
	if loggedUser == "" {
		return nil, errors.New("student identity unavailable")
	}
	form := url.Values{"gpid": {strconv.Itoa(id + 1)}, "gsh": {string(gsh[1])}, "action": {"loadData"}, "user": {loggedUser}, "changes": {"{}"}, "date": {date.Format("2006-01-02")}, "dateto": {date.Format("2006-01-02")}, "_LJSL": {"4096"}}
	req, _ = http.NewRequestWithContext(ctx, "POST", "https://"+client.Credentials.Server+"/gcall", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Credentials.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	marker := []byte(loggedUser + `",`)
	at := bytes.Index(body, marker)
	if at < 0 {
		return nil, errors.New("lesson plan response unavailable")
	}
	rest := bytes.TrimLeft(body[at+len(marker):], " \r\n")
	var plan struct {
		Dates map[string]struct {
			Plan []map[string]interface{} `json:"plan"`
		} `json:"dates"`
	}
	if err = json.NewDecoder(bytes.NewReader(rest)).Decode(&plan); err != nil {
		return nil, fmt.Errorf("lesson plan could not be read")
	}
	day, ok := plan.Dates[date.Format("2006-01-02")]
	if !ok {
		return []map[string]interface{}{}, nil
	}
	return day.Plan, nil
}
