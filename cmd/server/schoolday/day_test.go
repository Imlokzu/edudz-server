package schoolday

import (
	"testing"
	"time"

	"github.com/DislikesSchool/EduPage2-server/edupage/model"
)

func bells() []model.Period {
	return []model.Period{
		{ID: "1", StartTime: "08:00", EndTime: "08:45"},
		{ID: "2", StartTime: "08:45", EndTime: "09:30"},
		{ID: "3", StartTime: "09:45", EndTime: "10:30"},
		{ID: "4", StartTime: "10:30", EndTime: "11:15"},
		{ID: "5", StartTime: "11:30", EndTime: "12:15"},
		{ID: "6", StartTime: "12:15", EndTime: "13:00"},
		{ID: "7", StartTime: "13:00", EndTime: "13:45"},
		{ID: "8", StartTime: "14:00", EndTime: "14:45"},
		{ID: "9", StartTime: "14:45", EndTime: "15:30"},
	}
}
func lesson(period, start, end string) model.TimetableItem {
	return model.TimetableItem{Period: period, StartTime: start, EndTime: end, SubjectID: "subject", StudentIDs: []string{"student"}, TeacherIDs: []string{"teacher"}, ClassroomIDs: []string{"room"}}
}
func schoolSlots() []Slot {
	return Split([]model.TimetableItem{lesson("1", "08:00", "09:30"), lesson("3", "09:45", "10:30"), lesson("4", "10:30", "11:15"), lesson("5", "11:30", "13:00")}, bells())
}
func TestDoubleLessonsAndRealBreaks(t *testing.T) {
	slots := schoolSlots()
	if len(slots) != 6 {
		t.Fatalf("wanted six lessons, got %d", len(slots))
	}
	for i, s := range slots {
		if Minute(s.EndTime)-Minute(s.StartTime) != 45 {
			t.Fatalf("lesson %d is not 45 minutes", i+1)
		}
		if s.TeacherIDs[0] != "teacher" || s.ClassroomIDs[0] != "room" {
			t.Fatal("lost lesson details")
		}
	}
	if slots[1].Period != "2" || slots[1].OriginPeriod != "1" || slots[1].BlockStart != "08:00" || slots[1].BlockEnd != "09:30" {
		t.Fatal("lost double lesson identity")
	}
	gaps := Gaps(slots, bells())
	if len(gaps) != 2 || gaps[0].Start != "09:30" || gaps[0].End != "09:45" || gaps[1].Start != "11:15" || gaps[1].End != "11:30" {
		t.Fatalf("incorrect breaks: %+v", gaps)
	}
	if gaps[0].Kind != "break" || gaps[0].Seconds != 900 {
		t.Fatal("incorrect break duration")
	}
}
func TestSchoolClockBoundaries(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 10, 7, 0, 0, 0, 0, zone)
	for _, tc := range []struct {
		at, phase          string
		current, remaining int
		seconds            int64
	}{
		{"07:59:59", "before_school", -1, 6, 1},
		{"08:00:00", "lesson", 0, 6, 2700},
		{"08:44:59", "lesson", 0, 6, 1},
		{"08:45:00", "lesson", 1, 5, 2700},
		{"09:30:00", "break", -1, 4, 900},
		{"09:45:00", "lesson", 2, 4, 2700},
		{"11:15:00", "break", -1, 2, 900},
		{"11:30:00", "lesson", 4, 2, 2700},
		{"12:15:00", "lesson", 5, 1, 2700},
		{"13:00:00", "after_school", -1, 0, 0},
	} {
		t.Run(tc.at, func(t *testing.T) {
			now, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-07 "+tc.at, zone)
			s := Evaluate(date, now, schoolSlots())
			if s.Phase != tc.phase || s.Remaining != tc.remaining || s.Seconds != tc.seconds || s.SchoolStart != "08:00" || s.SchoolEnd != "13:00" {
				t.Fatalf("unexpected state: %+v", s)
			}
			if tc.current >= 0 && (s.CurrentIndex == nil || *s.CurrentIndex != tc.current) {
				t.Fatal("wrong current lesson")
			}
			if tc.current < 0 && s.CurrentIndex != nil {
				t.Fatal("unexpected current lesson")
			}
		})
	}
}
func TestFreePeriodAndFallback(t *testing.T) {
	slots := Split([]model.TimetableItem{lesson("5", "11:30", "13:00"), lesson("8", "14:00", "15:30")}, bells())
	gaps := Gaps(slots, bells())
	if len(slots) != 4 || len(gaps) != 1 || gaps[0].Kind != "free_period" || gaps[0].Seconds != 3600 {
		t.Fatalf("free period was counted as a lesson: %+v", gaps)
	}
	fallback := Split([]model.TimetableItem{lesson("3", "09:45", "11:15")}, nil)
	if len(fallback) != 2 || fallback[1].Period != "4" || fallback[1].StartTime != "10:30" {
		t.Fatal("fallback does not split doubles")
	}
	invalid := lesson("0", "broken", "11:15")
	placeholder := lesson("7", "13:00", "13:45")
	placeholder.StudentIDs = nil
	if len(Split([]model.TimetableItem{invalid, placeholder}, bells())) != 0 {
		t.Fatal("placeholder became a lesson")
	}
}
func TestNextDaySkipsWeekendsAndLongHolidays(t *testing.T) {
	days := map[string][]Slot{"2026-10-09": schoolSlots(), "2026-10-10": {}, "2026-10-11": {}, "2026-10-12": schoolSlots()}
	if NextDate("2026-10-09", days) != "2026-10-12" {
		t.Fatal("did not skip weekend")
	}
	delete(days, "2026-10-12")
	days["2026-11-23"] = schoolSlots()
	if NextDate("2026-10-09", days) != "2026-11-23" {
		t.Fatal("did not skip holidays")
	}
	if NextDate("2026-11-23", days) != "" {
		t.Fatal("invented a future school day")
	}
	if Evaluate(time.Now(), time.Now(), nil).Phase != "no_school" {
		t.Fatal("empty day is not a day off")
	}
}
