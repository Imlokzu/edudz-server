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
		if key > date && len(Active(slots)) > 0 && (result == "" || key < result) {
			result = key
		}
	}
	return result
}

type Slot struct {
	model.TimetableItem
	BlockStart   string  `json:"block_starttime"`
	BlockEnd     string  `json:"block_endtime"`
	OriginPeriod string  `json:"origin_period"`
	Changes      Changes `json:"-"`
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
	Cancelled    int        `json:"cancelled_lessons"`
	BreakKind    string     `json:"break_kind,omitempty"`
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
		isNamedEvent := item.Type == "event" && item.Name != ""
		if start < 0 || end <= start || (!item.IsCancelled() && (item.StudentIDs == nil || (item.SubjectID == "" && !isNamedEvent))) {
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
			result = append(result, Slot{TimetableItem: copy, BlockStart: item.StartTime, BlockEnd: item.EndTime, OriginPeriod: item.Period, Changes: Changes{Changed: item.Changed, Cancelled: item.IsCancelled()}})
		}
		// Use bell periods only when they cover the block's full start/end boundaries.
		if len(matched) > 0 && matched[0].StartTime == item.StartTime && matched[len(matched)-1].EndTime == item.EndTime {
			for _, p := range matched {
				if isNamedEvent {
					inBreak := false
					for _, pause := range DefaultBreaks() {
						if Minute(p.StartTime) >= Minute(pause.Start) && Minute(p.EndTime) <= Minute(pause.End) {
							inBreak = true
							break
						}
					}
					if inBreak {
						continue
					}
				}
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
func DefaultBreaks() []Break {
	return []Break{{"09:30", "09:45", 900, "break"}, {"11:15", "11:30", 900, "break"}, {"13:00", "14:00", 3600, "break"}}
}
func Gaps(slots []Slot, periods []model.Period, schedules ...[]Break) []Break {
	scheduled := DefaultBreaks()
	if len(schedules) > 0 {
		scheduled = schedules[0]
	}
	slots = Active(slots)
	result := []Break{}
	if len(slots) == 0 {
		return result
	}
	end := Minute(slots[0].EndTime)
	for _, slot := range slots[1:] {
		start, nextEnd := Minute(slot.StartTime), Minute(slot.EndTime)
		if start > end {
			cuts := []int{end, start}
			for _, pause := range scheduled {
				from, to := Minute(pause.Start), Minute(pause.End)
				if from > end && from < start {
					cuts = append(cuts, from)
				}
				if to > end && to < start {
					cuts = append(cuts, to)
				}
			}
			sort.Ints(cuts)
			for i := 1; i < len(cuts); i++ {
				from, to := cuts[i-1], cuts[i]
				if to <= from {
					continue
				}
				kind := "break"
				known := false
				for _, pause := range scheduled {
					if from >= Minute(pause.Start) && to <= Minute(pause.End) {
						known = true
						break
					}
				}
				if !known {
					for _, p := range periods {
						if Minute(p.StartTime) >= from && Minute(p.EndTime) <= to && Minute(p.EndTime) > Minute(p.StartTime) {
							kind = "free_period"
							break
						}
					}
				}
				result = append(result, Break{Clock(from), Clock(to), (to - from) * 60, kind})
			}
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
func Evaluate(date, now time.Time, slots []Slot, gaps ...[]Break) State {
	active := []int{}
	for index, slot := range slots {
		if !slot.Changes.Cancelled {
			active = append(active, index)
		}
	}
	state := State{Phase: "no_school", Total: len(active), Cancelled: len(slots) - len(active)}
	if len(active) == 0 {
		return state
	}
	state.SchoolStart = slots[active[0]].StartTime
	state.SchoolEnd = slots[active[len(active)-1]].EndTime
	for position, index := range active {
		slot := slots[index]
		start, end := At(date, slot.StartTime), At(date, slot.EndTime)
		if !now.Before(end) {
			continue
		}
		state.Remaining = len(active) - position
		if now.Before(start) {
			next := index
			state.NextIndex = &next
			state.EndsAt = &start
			state.Phase = "break"
			state.BreakKind = "free_period"
			if position == 0 {
				state.Phase = "before_school"
				state.BreakKind = ""
			} else {
				ranges := Gaps(slots, nil)
				if len(gaps) > 0 {
					ranges = gaps[0]
				}
				for _, gap := range ranges {
					from, to := At(date, gap.Start), At(date, gap.End)
					if !now.Before(from) && now.Before(to) {
						state.BreakKind = gap.Kind
						state.EndsAt = &to
						break
					}
				}
			}
			state.Seconds = int64(state.EndsAt.Sub(now).Seconds())
			return state
		}
		current := index
		state.CurrentIndex = &current
		state.EndsAt = &end
		state.Phase = "lesson"
		state.Seconds = int64(end.Sub(now).Seconds())
		if position+1 < len(active) {
			next := active[position+1]
			state.NextIndex = &next
		}
		return state
	}
	state.Phase = "after_school"
	return state
}
