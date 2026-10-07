package schoolday

import (
	"sort"
	"strconv"
	"time"

	"github.com/DislikesSchool/EduPage2-server/edupage/model"
)

const LookAheadDays = 60

// NextDate skips days without actual lessons, including holidays and weekends.
func NextDate(date string, days map[string][]Slot) string {
	result := ""
	for key, slots := range days {
		if key > date && len(slots) > 0 && (result == "" || key < result) {
			result = key
		}
	}
	return result
}

type Slot struct {
	model.TimetableItem
	BlockStart   string `json:"block_starttime"`
	BlockEnd     string `json:"block_endtime"`
	OriginPeriod string `json:"origin_period"`
}
type Break struct {
	Start   string `json:"starttime"`
	End     string `json:"endtime"`
	Seconds int    `json:"duration_seconds"`
	Kind    string `json:"kind"`
}
type State struct {
	Phase        string     `json:"phase"`
	SchoolStart  string     `json:"school_start"`
	SchoolEnd    string     `json:"school_end"`
	Total        int        `json:"total_lessons"`
	Remaining    int        `json:"remaining_lessons"`
	CurrentIndex *int       `json:"current_index"`
	NextIndex    *int       `json:"next_index"`
	Seconds      int64      `json:"seconds_remaining"`
	EndsAt       *time.Time `json:"phase_ends_at,omitempty"`
}

func Minute(value string) int {
	if len(value) < 4 {
		return -1
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return -1
	}
	return parsed.Hour()*60 + parsed.Minute()
}
func Clock(value int) string {
	return time.Date(2000, 1, 1, value/60, value%60, 0, 0, time.UTC).Format("15:04")
}
func Split(items []model.TimetableItem, periods []model.Period) []Slot {
	sort.Slice(periods, func(i, j int) bool { return Minute(periods[i].StartTime) < Minute(periods[j].StartTime) })
	result := []Slot{}
	for _, item := range items {
		start, end := Minute(item.StartTime), Minute(item.EndTime)
		if start < 0 || end <= start || item.StudentIDs == nil || item.SubjectID == "" {
			continue
		}
		matched := []model.Period{}
		for _, p := range periods {
			ps, pe := Minute(p.StartTime), Minute(p.EndTime)
			if ps >= start && pe <= end && pe > ps {
				matched = append(matched, p)
			}
		}
		appendSlot := func(period, from, to string) {
			copy := item
			copy.Period = period
			copy.StartTime = from
			copy.EndTime = to
			result = append(result, Slot{copy, item.StartTime, item.EndTime, item.Period})
		}
		// Use bell periods only when they cover the block's full start/end boundaries.
		if len(matched) > 0 && matched[0].StartTime == item.StartTime && matched[len(matched)-1].EndTime == item.EndTime {
			for _, p := range matched {
				appendSlot(p.ID, p.StartTime, p.EndTime)
			}
		} else if (end-start) > 45 && (end-start)%45 == 0 {
			number, err := strconv.Atoi(item.Period)
			for at := start; at < end; at += 45 {
				period := item.Period
				if err == nil {
					period = strconv.Itoa(number + (at-start)/45)
				}
				appendSlot(period, Clock(at), Clock(at+45))
			}
		} else {
			appendSlot(item.Period, item.StartTime, item.EndTime)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return Minute(result[i].StartTime) < Minute(result[j].StartTime) })
	return result
}
func Gaps(slots []Slot, periods []model.Period) []Break {
	result := []Break{}
	if len(slots) == 0 {
		return result
	}
	end := Minute(slots[0].EndTime)
	for _, slot := range slots[1:] {
		start, nextEnd := Minute(slot.StartTime), Minute(slot.EndTime)
		if start > end {
			kind := "break"
			for _, p := range periods {
				if Minute(p.StartTime) >= end && Minute(p.EndTime) <= start && Minute(p.EndTime) > Minute(p.StartTime) {
					kind = "free_period"
					break
				}
			}
			result = append(result, Break{Clock(end), Clock(start), (start - end) * 60, kind})
		}
		if nextEnd > end {
			end = nextEnd
		}
	}
	return result
}
func At(date time.Time, clock string) time.Time {
	minute := Minute(clock)
	return time.Date(date.Year(), date.Month(), date.Day(), minute/60, minute%60, 0, 0, date.Location())
}
func Evaluate(date, now time.Time, slots []Slot) State {
	state := State{Phase: "no_school", Total: len(slots)}
	if len(slots) == 0 {
		return state
	}
	state.SchoolStart = slots[0].StartTime
	state.SchoolEnd = slots[len(slots)-1].EndTime
	for i, slot := range slots {
		start, end := At(date, slot.StartTime), At(date, slot.EndTime)
		if !now.Before(end) {
			continue
		}
		state.Remaining = len(slots) - i
		index := i
		if now.Before(start) {
			state.NextIndex = &index
			state.EndsAt = &start
			state.Phase = "break"
			if i == 0 {
				state.Phase = "before_school"
			}
			state.Seconds = int64(start.Sub(now).Seconds())
			return state
		}
		state.CurrentIndex = &index
		state.EndsAt = &end
		state.Phase = "lesson"
		state.Seconds = int64(end.Sub(now).Seconds())
		if i+1 < len(slots) {
			next := i + 1
			state.NextIndex = &next
		}
		return state
	}
	state.Phase = "after_school"
	return state
}
