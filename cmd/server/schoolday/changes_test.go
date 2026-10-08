package schoolday

import (
	"github.com/DislikesSchool/EduPage2-server/edupage/model"
	"testing"
	"time"
)

func publishedPlan(period, start, end string, cancelled bool) map[string]interface{} {
	return map[string]interface{}{"type": "lesson", "uniperiod": period, "starttime": start, "endtime": end, "flags": map[string]interface{}{"dp0": map[string]interface{}{
		"type": "card", "cancelled": cancelled, "subjectid": "subject", "teacherids": []interface{}{"teacher"}, "classroomids": []interface{}{"room"}, "classids": []interface{}{},
		"orig":    map[string]interface{}{"subjectid": "subject", "teacherids": []interface{}{"usual-teacher"}, "classroomids": []interface{}{"usual-room"}, "classids": []interface{}{}},
		"changes": []interface{}{map[string]interface{}{"column": "teacherids"}, map[string]interface{}{"column": "classroomids"}},
	}}}
}
func TestPublishedSubstitutionAndRoomForBothHalves(t *testing.T) {
	slots := schoolSlots()
	for i := range slots {
		slots[i].Changes.Changed = true
	}
	result := Enrich(slots, []map[string]interface{}{publishedPlan("1", "08:00", "08:45", false), publishedPlan("2", "08:45", "09:30", false)})
	for _, slot := range result[:2] {
		if !slot.Changes.Teacher || !slot.Changes.Room || slot.Changes.Cancelled {
			t.Fatalf("missing substitution: %+v", slot.Changes)
		}
		if slot.Changes.OriginalTeachers[0] != "usual-teacher" || slot.Changes.OriginalRooms[0] != "usual-room" {
			t.Fatal("original details lost")
		}
	}
	if result[2].Changes.Teacher || result[2].Changes.Room {
		t.Fatal("guessed an unconfirmed change")
	}
	if slots[0].Changes.Teacher {
		t.Fatal("mutated cached source")
	}
	ambiguous := Enrich(slots, []map[string]interface{}{publishedPlan("1", "08:00", "08:45", false), publishedPlan("1", "08:00", "08:45", false)})
	if ambiguous[0].Changes.Teacher || ambiguous[0].Changes.Room {
		t.Fatal("ambiguous match classified")
	}
}
func TestCancelledLessonsStayVisibleButDoNotCount(t *testing.T) {
	slots := Enrich(schoolSlots(), []map[string]interface{}{publishedPlan("5", "11:30", "12:15", true), publishedPlan("6", "12:15", "13:00", true)})
	if len(slots) != 6 || len(Active(slots)) != 4 {
		t.Fatal("cancelled lessons must remain visible")
	}
	date := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	state := Evaluate(date, At(date, "11:15"), slots)
	if state.Total != 4 || state.Cancelled != 2 || state.Remaining != 0 || state.SchoolEnd != "11:15" || state.Phase != "after_school" {
		t.Fatalf("cancelled lesson affected clock: %+v", state)
	}
	for i := range slots {
		slots[i].Changes.Cancelled = true
	}
	if Evaluate(date, At(date, "08:00"), slots).Phase != "no_school" {
		t.Fatal("all-cancelled day counts as school")
	}
	if NextDate("2026-10-06", map[string][]Slot{"2026-10-07": slots, "2026-10-08": schoolSlots()}) != "2026-10-08" {
		t.Fatal("did not skip all-cancelled day")
	}
	removed := lesson("1", "08:00", "09:30")
	removed.Removed = true
	removed.StudentIDs = nil
	removed.SubjectID = ""
	if len(Split([]model.TimetableItem{removed}, bells())) != 2 {
		t.Fatal("redacted removed double disappeared")
	}
}
func TestFreeTimeSeparatesKnownBreaks(t *testing.T) {
	slots := Split([]model.TimetableItem{lesson("1", "08:00", "08:45"), lesson("5", "11:30", "12:15")}, bells())
	gaps := Gaps(slots, bells())
	want := []Break{{"08:45", "09:30", 2700, "free_period"}, {"09:30", "09:45", 900, "break"}, {"09:45", "11:15", 5400, "free_period"}, {"11:15", "11:30", 900, "break"}}
	if len(gaps) != len(want) {
		t.Fatalf("missing break segments: %+v", gaps)
	}
	for i, gap := range gaps {
		if gap != want[i] {
			t.Fatalf("bad segment %+v", gap)
		}
	}
	date := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	s := Evaluate(date, At(date, "09:35"), slots, gaps)
	if s.BreakKind != "break" || s.Seconds != 600 || s.Remaining != 1 {
		t.Fatalf("wrong school-break clock: %+v", s)
	}
	s = Evaluate(date, At(date, "09:45"), slots, gaps)
	if s.BreakKind != "free_period" || s.Seconds != 5400 {
		t.Fatal("break did not end at 09:45")
	}
}

func TestPartialOriginalDoesNotInventRoomOrSubjectChanges(t *testing.T) {
	plan := publishedPlan("1", "08:00", "08:45", false)
	dp := plan["flags"].(map[string]interface{})["dp0"].(map[string]interface{})
	dp["orig"] = map[string]interface{}{"teacherids": []interface{}{"usual-teacher"}}
	dp["changes"] = []interface{}{}
	change := Enrich(schoolSlots(), []map[string]interface{}{plan})[0].Changes
	if !change.Teacher || change.Room || change.Class || change.Subject {
		t.Fatalf("invented a difference: %+v", change)
	}
}
func TestPublishedClassAndSubjectChanges(t *testing.T) {
	item := lesson("1", "08:00", "08:45")
	item.SubjectID = "new-subject"
	item.ClassIDs = []string{"new-class"}
	plan := publishedPlan("1", "08:00", "08:45", false)
	dp := plan["flags"].(map[string]interface{})["dp0"].(map[string]interface{})
	dp["subjectid"] = "new-subject"
	dp["classids"] = []interface{}{"new-class"}
	orig := dp["orig"].(map[string]interface{})
	orig["subjectid"] = "old-subject"
	orig["classids"] = []interface{}{"old-class"}
	change := Enrich(Split([]model.TimetableItem{item}, bells()), []map[string]interface{}{plan})[0].Changes
	if !change.Subject || !change.Class || change.OriginalSubject != "old-subject" || change.OriginalClasses[0] != "old-class" {
		t.Fatal("lost published class/subject difference")
	}
}
func TestReplacementEventKeepsTheActualSchoolTime(t *testing.T) {
	removed := lesson("1", "08:00", "09:30")
	removed.Removed = true
	removed.StudentIDs = nil
	removed.SubjectID = ""
	event := lesson("1", "08:00", "12:15")
	event.Type = "event"
	event.Name = "School trip"
	event.SubjectID = ""
	slots := Split([]model.TimetableItem{removed, event}, bells())
	date := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	s := Evaluate(date, At(date, "08:20"), slots)
	if s.Total != 5 || s.Cancelled != 2 || s.SchoolStart != "08:00" || s.SchoolEnd != "12:15" || s.Phase != "lesson" {
		t.Fatalf("event replacement affected attendance: %+v", s)
	}
}
