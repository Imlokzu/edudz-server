package routes

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DislikesSchool/EduPage2-server/cmd/server/apimodel"
	"github.com/DislikesSchool/EduPage2-server/cmd/server/schoolday"
	"github.com/DislikesSchool/EduPage2-server/edupage"
	"github.com/DislikesSchool/EduPage2-server/edupage/model"
	"github.com/gin-gonic/gin"
)

type schoolDayCacheEntry struct {
	Timetable model.Timetable
	Plans     map[string][]map[string]interface{}
	Cached    time.Time
}

var schoolDayCache = struct {
	sync.Mutex
	Entries map[string]schoolDayCacheEntry
}{Entries: map[string]schoolDayCacheEntry{}}

func completeSchoolSlots(slots []schoolday.Slot, user model.User) []apimodel.CompleteTimetableItem {
	result := []apimodel.CompleteTimetableItem{}
	for _, slot := range slots {
		item := slot.TimetableItem
		value := apimodel.CompleteTimetableItem{Type: item.Type, Date: item.Date, Period: item.Period, StartTime: item.StartTime, EndTime: item.EndTime, Subject: user.DBI.Subjects[item.SubjectID], GroupNames: item.GroupNames, IGroupID: item.IGroupID, StudentIDs: item.StudentIDs, Colors: item.Colors, BlockStart: slot.BlockStart, BlockEnd: slot.BlockEnd, OriginPeriod: slot.OriginPeriod, Classes: []model.Class{}, Teachers: []model.Teacher{}, Classrooms: []model.Classroom{}}
		value.Changes = completeLessonChanges(slot.Changes, user)
		if item.Type == "event" && value.Subject.Name == "" {
			value.Subject.Name = item.Name
			value.Subject.Short = item.Name
		}
		if value.StudentIDs == nil {
			value.StudentIDs = []string{}
		}
		if value.GroupNames == nil {
			value.GroupNames = []string{}
		}
		if value.Colors == nil {
			value.Colors = []string{}
		}
		for _, id := range item.ClassIDs {
			value.Classes = append(value.Classes, user.DBI.Classes[id])
		}
		for _, id := range item.TeacherIDs {
			value.Teachers = append(value.Teachers, user.DBI.Teachers[id])
		}
		for _, id := range item.ClassroomIDs {
			value.Classrooms = append(value.Classrooms, user.DBI.Classrooms[id])
		}
		result = append(result, value)
	}
	return result
}

func completeLessonChanges(changes schoolday.Changes, user model.User) apimodel.LessonChanges {
	result := apimodel.LessonChanges{Changed: changes.Changed, Cancelled: changes.Cancelled, Teacher: changes.Teacher, Room: changes.Room, Class: changes.Class, Subject: changes.Subject, OriginalSubject: user.DBI.Subjects[changes.OriginalSubject].Name}
	teachers, rooms, classes := []string{}, []string{}, []string{}
	for _, id := range changes.OriginalTeachers {
		teacher := user.DBI.Teachers[id]
		name := strings.TrimSpace(teacher.Firstname + " " + teacher.Lastname)
		if name != "" {
			teachers = append(teachers, name)
		}
	}
	for _, id := range changes.OriginalRooms {
		if room := user.DBI.Classrooms[id].Name; room != "" {
			rooms = append(rooms, room)
		}
	}
	for _, id := range changes.OriginalClasses {
		if class := user.DBI.Classes[id].Name; class != "" {
			classes = append(classes, class)
		}
	}
	result.OriginalTeacher = strings.Join(teachers, ", ")
	result.OriginalRoom = strings.Join(rooms, ", ")
	result.OriginalClass = strings.Join(classes, ", ")
	return result
}
func SchoolDayHandler(c *gin.Context) {
	client := c.MustGet("client").(*edupage.EdupageClient)
	user, err := client.GetUser(false)
	if err != nil {
		c.JSON(502, gin.H{"error": "school_day_unavailable"})
		return
	}
	zone := os.Getenv("EDUDZ_SCHOOL_TIMEZONE")
	if zone == "" {
		zone, _ = user.Edubar["timezone"].(string)
	}
	if zone == "" {
		zone = "Europe/Berlin"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		c.JSON(500, gin.H{"error": "school_timezone_unavailable"})
		return
	}
	now := time.Now().In(location)
	dateString := c.Query("date")
	if dateString == "" {
		dateString = now.Format("2006-01-02")
	}
	date, err := time.ParseInLocation("2006-01-02", dateString, location)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid_date"})
		return
	}
	// Time overrides are read-only and allow deterministic schedule previews.
	if at := c.Query("at"); at != "" {
		parsed, e := time.Parse(time.RFC3339, at)
		if e != nil {
			c.JSON(400, gin.H{"error": "invalid_time"})
			return
		}
		now = parsed.In(location)
	}
	key := client.Credentials.Username + "@" + client.Credentials.Server + ":" + dateString
	schoolDayCache.Lock()
	cached, ok := schoolDayCache.Entries[key]
	schoolDayCache.Unlock()
	var timetable model.Timetable
	plans := map[string][]map[string]interface{}{}
	if ok && time.Since(cached.Cached) < time.Minute {
		timetable = cached.Timetable
		plans = cached.Plans
	} else {
		timetable, err = client.GetTimetable(date, date.AddDate(0, 0, schoolday.LookAheadDays))
		if err != nil {
			c.JSON(502, gin.H{"error": "school_day_unavailable"})
			return
		}
		enrichmentCtx, stopEnrichment := context.WithTimeout(c.Request.Context(), 12*time.Second)
		defer stopEnrichment()
		for day, items := range timetable.Days {
			if enrichmentCtx.Err() != nil {
				break
			}
			needsPlan := false
			for _, item := range items {
				if item.Changed || item.IsCancelled() {
					needsPlan = true
					break
				}
			}
			if !needsPlan {
				continue
			}
			dayDate, e := time.ParseInLocation("2006-01-02", day, location)
			if e != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(enrichmentCtx, 4*time.Second)
			plan, e := client.LessonPlan(ctx, dayDate)
			cancel()
			if e == nil {
				plans[day] = plan
			}
		}
		schoolDayCache.Lock()
		if len(schoolDayCache.Entries) > 500 {
			for k, v := range schoolDayCache.Entries {
				if time.Since(v.Cached) > time.Minute {
					delete(schoolDayCache.Entries, k)
				}
			}
		}
		if len(schoolDayCache.Entries) < 500 || ok {
			schoolDayCache.Entries[key] = schoolDayCacheEntry{Timetable: timetable, Plans: plans, Cached: time.Now()}
		}
		schoolDayCache.Unlock()
	}
	periods := []model.Period{}
	for _, p := range user.DBI.Periods {
		periods = append(periods, p)
	}
	slots := schoolday.Enrich(schoolday.Split(timetable.Days[dateString], periods), plans[dateString])
	breaks := schoolday.Gaps(slots, periods)
	state := schoolday.Evaluate(date, now, slots, breaks)
	all := map[string][]apimodel.CompleteTimetableItem{}
	days := map[string][]schoolday.Slot{}
	for offset := 0; offset <= schoolday.LookAheadDays; offset++ {
		day := date.AddDate(0, 0, offset)
		dayKey := day.Format("2006-01-02")
		daySlots := schoolday.Enrich(schoolday.Split(timetable.Days[dayKey], periods), plans[dayKey])
		days[dayKey] = daySlots
		all[dayKey] = completeSchoolSlots(daySlots, user)
	}
	nextDate := schoolday.NextDate(dateString, days)
	var next interface{}
	if nextDate != "" {
		nextSlots := schoolday.Active(days[nextDate])
		next = gin.H{"date": nextDate, "school_start": nextSlots[0].StartTime, "school_end": nextSlots[len(nextSlots)-1].EndTime, "total_lessons": len(nextSlots), "lessons": all[nextDate]}
	}
	displayDate := dateString
	if state.Phase == "after_school" || state.Phase == "no_school" {
		if nextDate != "" {
			displayDate = nextDate
		}
	}
	_, offset := now.Zone()
	c.JSON(http.StatusOK, gin.H{"date": dateString, "display_date": displayDate, "server_time": now.Format(time.RFC3339Nano), "time_zone": zone, "utc_offset_seconds": offset, "state": state, "lessons": completeSchoolSlots(slots, user), "breaks": breaks, "scheduled_breaks": schoolday.DefaultBreaks(), "next_school_day": next, "days": all, "periods": user.DBI.Periods, "checked_until": date.AddDate(0, 0, schoolday.LookAheadDays).Format("2006-01-02")})
}
