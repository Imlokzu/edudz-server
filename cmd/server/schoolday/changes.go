package schoolday

import (
	"fmt"
	"sort"
	"strings"
)

// Changes only carries differences published by the school's day plan.
type Changes struct {
	Changed                                          bool
	Cancelled                                        bool
	Teacher                                          bool
	Room                                             bool
	Class                                            bool
	Subject                                          bool
	OriginalSubject                                  string
	OriginalTeachers, OriginalRooms, OriginalClasses []string
}

func ids(value interface{}) []string {
	result := []string{}
	if list, ok := value.([]interface{}); ok {
		for _, id := range list {
			text := fmt.Sprint(id)
			if text != "" && text != "<nil>" {
				result = append(result, text)
			}
		}
	}
	return result
}
func sameIDs(a, b []string) bool {
	a = append([]string{}, a...)
	b = append([]string{}, b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
func text(value interface{}) string { v, _ := value.(string); return v }
func intersects(a, b []string) bool {
	for _, one := range a {
		for _, two := range b {
			if one == two {
				return true
			}
		}
	}
	return false
}

// Enrich matches exact bell times and the student's class/subject. Ambiguous
// matches keep the generic changed flag instead of guessing a change type.
func Enrich(slots []Slot, plans []map[string]interface{}) []Slot {
	result := append([]Slot{}, slots...)
	for index := range result {
		slot := &result[index]
		if slot.Type == "event" {
			continue
		}
		matches := []map[string]interface{}{}
		for _, plan := range plans {
			if text(plan["starttime"]) != slot.StartTime || text(plan["endtime"]) != slot.EndTime {
				continue
			}
			flags, _ := plan["flags"].(map[string]interface{})
			dp, _ := flags["dp0"].(map[string]interface{})
			if dp == nil || text(dp["type"]) == "event" || text(plan["type"]) == "break" {
				continue
			}
			if cancelled, _ := dp["cancelled"].(bool); slot.Changes.Cancelled && !cancelled {
				continue
			}
			orig, _ := dp["orig"].(map[string]interface{})
			subject := text(dp["subjectid"])
			if slot.SubjectID != "" && subject != slot.SubjectID && text(orig["subjectid"]) != slot.SubjectID {
				continue
			}
			classes := ids(dp["classids"])
			if len(slot.ClassIDs) > 0 && len(classes) > 0 && !intersects(slot.ClassIDs, classes) && !intersects(slot.ClassIDs, ids(orig["classids"])) {
				continue
			}
			matches = append(matches, dp)
		}
		if len(matches) != 1 {
			continue
		}
		dp := matches[0]
		orig, _ := dp["orig"].(map[string]interface{})
		if cancelled, _ := dp["cancelled"].(bool); cancelled {
			slot.Changes.Cancelled = true
		}
		if orig != nil {
			slot.Changes.OriginalSubject = text(orig["subjectid"])
			slot.Changes.OriginalTeachers = ids(orig["teacherids"])
			slot.Changes.OriginalRooms = ids(orig["classroomids"])
			slot.Changes.OriginalClasses = ids(orig["classids"])
			if _, known := orig["teacherids"]; known {
				slot.Changes.Teacher = !sameIDs(slot.Changes.OriginalTeachers, ids(dp["teacherids"]))
			}
			if _, known := orig["classroomids"]; known {
				slot.Changes.Room = !sameIDs(slot.Changes.OriginalRooms, ids(dp["classroomids"]))
			}
			if _, known := orig["classids"]; known {
				slot.Changes.Class = !sameIDs(slot.Changes.OriginalClasses, ids(dp["classids"]))
			}
			if _, known := orig["subjectid"]; known {
				slot.Changes.Subject = slot.Changes.OriginalSubject != text(dp["subjectid"])
			}
		}
		if list, ok := dp["changes"].([]interface{}); ok {
			for _, raw := range list {
				change, _ := raw.(map[string]interface{})
				switch text(change["column"]) {
				case "teacherids":
					slot.Changes.Teacher = true
				case "classroomids":
					slot.Changes.Room = true
				case "classids":
					slot.Changes.Class = true
				case "subjectid":
					slot.Changes.Subject = true
				}
			}
		}
		slot.Changes.Changed = slot.Changes.Changed || slot.Changes.Teacher || slot.Changes.Room || slot.Changes.Class || slot.Changes.Subject
	}
	return result
}

func Active(slots []Slot) []Slot {
	result := []Slot{}
	for _, slot := range slots {
		if !slot.Changes.Cancelled {
			result = append(result, slot)
		}
	}
	return result
}
