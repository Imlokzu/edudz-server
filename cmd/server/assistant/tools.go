package assistant

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DislikesSchool/EduPage2-server/edupage"
	"github.com/DislikesSchool/EduPage2-server/edupage/model"
	"github.com/ledongthuc/pdf"
)

var tags = regexp.MustCompile(`<[^>]+>`)

func clean(v string) string {
	return html.UnescapeString(strings.TrimSpace(tags.ReplaceAllString(v, " ")))
}
func shorten(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return v
}
func stringArg(args map[string]interface{}, key string) string { v, _ := args[key].(string); return v }
func Files(v interface{}) []map[string]string {
	out := []map[string]string{}
	switch value := v.(type) {
	case map[string]interface{}:
		if src, ok := value["src"].(string); ok {
			out = append(out, map[string]string{"src": src, "name": stringArg(value, "name")})
		} else if src, ok := value["url"].(string); ok {
			out = append(out, map[string]string{"src": src, "name": stringArg(value, "name")})
		} else {
			for src, name := range value {
				if n, ok := name.(string); ok {
					out = append(out, map[string]string{"src": src, "name": n})
				}
			}
		}
	case []interface{}:
		for _, v := range value {
			out = append(out, Files(v)...)
		}
	}
	for _, f := range out {
		if f["name"] == "" {
			f["name"] = path.Base(strings.Split(f["src"], "?")[0])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["src"] < out[j]["src"] })
	return out
}
func blocks(value map[string]interface{}) ([]map[string]interface{}, []map[string]string) {
	material, _ := value["materialData"].(map[string]interface{})
	cards := []interface{}{}
	switch raw := material["cardsData"].(type) {
	case map[string]interface{}:
		for _, card := range raw {
			cards = append(cards, card)
		}
	case []interface{}:
		cards = raw
	}
	out := []map[string]interface{}{}
	files := []map[string]string{}
	for _, raw := range cards {
		card, _ := raw.(map[string]interface{})
		content := card["content"]
		if text, ok := content.(string); ok {
			json.Unmarshal([]byte(text), &content)
		}
		c, _ := content.(map[string]interface{})
		widgets, _ := c["widgets"].([]interface{})
		for _, raw := range widgets {
			w, _ := raw.(map[string]interface{})
			props, _ := w["props"].(map[string]interface{})
			text := ""
			for _, key := range []string{"_parsedHtmlText", "htmlText", "text", "question"} {
				if s, ok := props[key].(string); ok && s != "" {
					text = clean(s)
					break
				}
			}
			attachments := Files(props["files"])
			files = append(files, attachments...)
			if text != "" || len(attachments) > 0 {
				out = append(out, map[string]interface{}{"text": shorten(text, 16000), "files": attachments})
			}
		}
	}
	return out, files
}
func SchoolTools(client *edupage.EdupageClient) ToolRunner {
	return func(ctx context.Context, name string, args map[string]interface{}) (ToolResult, error) {
		if err := ctx.Err(); err != nil {
			return ToolResult{}, err
		}
		switch name {
		case "school_overview":
			return overview(client, stringArg(args, "date"))
		case "homework_details":
			return homework(client, stringArg(args, "id"))
		case "lesson_plan":
			date, err := time.Parse("2006-01-02", stringArg(args, "date"))
			if err != nil {
				return ToolResult{}, err
			}
			plans, err := client.LessonPlan(ctx, date)
			return ToolResult{Data: map[string]interface{}{"date": date.Format("2006-01-02"), "lessons": plans}}, err
		case "read_attachment":
			return attachment(ctx, client, stringArg(args, "src"), stringArg(args, "name"), args["page"])
		default:
			return ToolResult{}, errors.New("unknown school tool")
		}
	}
}
func overview(client *edupage.EdupageClient, dateString string) (ToolResult, error) {
	date, err := time.Parse("2006-01-02", dateString)
	if err != nil {
		return ToolResult{}, err
	}
	user, err := client.GetUser(false)
	if err != nil {
		return ToolResult{}, err
	}
	timeline, err := client.GetRecentTimeline()
	if err != nil {
		return ToolResult{}, err
	}
	tasks := []map[string]interface{}{}
	for _, h := range timeline.Homeworks {
		tasks = append(tasks, map[string]interface{}{"id": h.ID, "subject": h.LessonName, "title": clean(h.Name), "description": shorten(clean(h.Details), 2500), "due": h.DateTo, "files": Files(h.Attachments)})
	}
	sort.Slice(tasks, func(i, j int) bool { return fmt.Sprint(tasks[i]["due"]) < fmt.Sprint(tasks[j]["due"]) })
	if len(tasks) > 80 {
		tasks = tasks[:80]
	}
	messages := []map[string]interface{}{}
	for _, m := range timeline.Items {
		if m.ReactionTo == "" && m.Text != "" && m.Removed.String() != "1" {
			messages = append(messages, map[string]interface{}{"sender": m.UserName, "text": shorten(clean(m.Text), 1800), "date": m.TimeAdded})
		}
	}
	sort.Slice(messages, func(i, j int) bool { return fmt.Sprint(messages[i]["date"]) > fmt.Sprint(messages[j]["date"]) })
	if len(messages) > 35 {
		messages = messages[:35]
	}
	lessons := map[string]interface{}{}
	tt, ttErr := client.GetTimetable(date, date.AddDate(0, 0, 6))
	if ttErr == nil {
		for day, items := range tt.Days {
			rows := []map[string]interface{}{}
			for _, item := range items {
				rooms := []string{}
				teachers := []string{}
				for _, id := range item.ClassroomIDs {
					rooms = append(rooms, user.DBI.Classrooms[id].Name)
				}
				for _, id := range item.TeacherIDs {
					teacher := user.DBI.Teachers[id]
					teachers = append(teachers, teacher.Firstname+" "+teacher.Lastname)
				}
				rows = append(rows, map[string]interface{}{"subject": user.DBI.Subjects[item.SubjectID].Name, "subject_id": item.SubjectID, "period": item.Period, "start": item.StartTime, "end": item.EndTime, "rooms": rooms, "teachers": teachers})
			}
			lessons[day] = rows
		}
	}
	grades := []map[string]interface{}{}
	results, gradeErr := client.GetRecentResults()
	if gradeErr == nil {
		for _, g := range results.Events {
			grades = append(grades, map[string]interface{}{"subject": user.DBI.Subjects[g.SubjectID].Name, "value": g.Data, "description": g.EventName, "date": g.Date})
		}
		if len(grades) > 60 {
			grades = grades[:60]
		}
	}
	return ToolResult{Data: map[string]interface{}{"date": dateString, "homework": tasks, "messages": messages, "timetable": lessons, "grades": grades, "timetable_available": ttErr == nil, "grades_available": gradeErr == nil}}, nil
}
func homework(client *edupage.EdupageClient, id string) (ToolResult, error) {
	timeline, err := client.GetRecentTimeline()
	if err != nil {
		return ToolResult{}, err
	}
	var selected *model.Homework
	for _, h := range timeline.Homeworks {
		if h.ID == id || h.HomeworkID == id {
			copy := h
			selected = &copy
			break
		}
	}
	if selected == nil {
		return ToolResult{}, errors.New("homework not found")
	}
	h := selected
	files := Files(h.Attachments)
	data := map[string]interface{}{"id": h.ID, "title": clean(h.Name), "subject": h.LessonName, "description": clean(h.Details), "due": h.DateTo}
	if h.TestID != "" && h.ESuperID != "" {
		etest, e := client.FetchETestData(h.TestID, h.ESuperID)
		if e == nil {
			cards, more := blocks(etest)
			data["materials"] = cards
			files = append(files, more...)
		} else {
			data["materials_available"] = false
		}
	}
	data["files"] = files
	return ToolResult{Data: data, Files: files}, nil
}
func attachment(ctx context.Context, client *edupage.EdupageClient, src, name string, pageArg interface{}) (result ToolResult, err error) {
	defer func() {
		if recover() != nil {
			result = ToolResult{}
			err = errors.New("attachment could not be parsed")
		}
	}()
	if src == "" || len(src) > 4096 {
		return ToolResult{}, errors.New("invalid attachment")
	}
	resp, err := client.FetchFileContext(ctx, src)
	if err != nil {
		return ToolResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ToolResult{}, errors.New("attachment unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil || len(body) > 16<<20 {
		return ToolResult{}, errors.New("attachment too large")
	}
	if name == "" {
		name = path.Base(strings.Split(src, "?")[0])
	}
	mime := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if mime == "" {
		mime = http.DetectContentType(body)
	}
	result.Files = []map[string]string{{"src": src, "name": name}}
	data := map[string]interface{}{"name": name, "content_type": mime}
	if strings.HasPrefix(mime, "image/") {
		if len(body) > 5<<20 {
			return ToolResult{}, errors.New("image too large")
		}
		result.Image = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(body)
		data["image_available"] = true
	} else if bytes.HasPrefix(body, []byte("%PDF-")) {
		r, e := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
		if e != nil {
			return ToolResult{}, e
		}
		reader, e := r.GetPlainText()
		if e != nil {
			return ToolResult{}, e
		}
		text, e := io.ReadAll(io.LimitReader(reader, 64000))
		if e != nil {
			return ToolResult{}, e
		}
		data["text"] = string(text)
		data["pages"] = r.NumPage()
		page := 1
		if v, ok := pageArg.(float64); ok && v >= 1 && v <= float64(r.NumPage()) {
			page = int(v)
		}
		images, rasterErr := renderPDF(ctx, body, page)
		if rasterErr == nil {
			result.Images = images
			data["viewed_pages_start"] = page
			data["viewed_pages_count"] = len(images)
		}
		if strings.TrimSpace(string(text)) == "" {
			data["note"] = "This PDF has no extractable text; do not invent its contents."
		}
	} else if strings.HasSuffix(strings.ToLower(name), ".docx") || strings.HasSuffix(strings.ToLower(name), ".pptx") || strings.HasSuffix(strings.ToLower(name), ".xlsx") {
		text, e := office(body)
		if e != nil {
			return ToolResult{}, e
		}
		data["text"] = shorten(text, 40000)
	} else if strings.HasPrefix(mime, "text/") || strings.HasSuffix(strings.ToLower(name), ".txt") {
		data["text"] = shorten(string(body), 40000)
	} else {
		data["note"] = "This attachment can be downloaded inside edudz, but text extraction is unavailable for its format."
	}
	result.Data = data
	return result, nil
}
func office(body []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", err
	}
	var result strings.Builder
	for _, file := range reader.File {
		if !(file.Name == "word/document.xml" || file.Name == "xl/sharedStrings.xml" || strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml")) {
			continue
		}
		if file.UncompressedSize64 > 8<<20 {
			return "", errors.New("office document too large")
		}
		input, e := file.Open()
		if e != nil {
			return "", e
		}
		decoder := xml.NewDecoder(io.LimitReader(input, 8<<20))
		inText := false
		for {
			token, e := decoder.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				input.Close()
				return "", e
			}
			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local == "t" {
					inText = true
				}
			case xml.EndElement:
				if t.Name.Local == "t" {
					inText = false
				}
				if t.Name.Local == "p" || t.Name.Local == "si" {
					result.WriteString("\n")
				}
			case xml.CharData:
				if inText {
					result.Write(t)
				}
			}
			if result.Len() > 64000 {
				break
			}
		}
		input.Close()
		if result.Len() > 64000 {
			break
		}
	}
	return result.String(), nil
}
