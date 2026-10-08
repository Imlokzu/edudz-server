package model

import "testing"

func TestTimetableKeepsSchoolChangeAndCancellationFlags(t *testing.T) {
	value, err := ParseTimetable([]byte(`{"r":{"ttitems":[{"date":"2026-10-08","type":"card","changed":true},{"date":"2026-10-08","type":"card","removed":true},{"date":"2026-10-08","type":"absent"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	items := value.Days["2026-10-08"]
	if !items[0].Changed || items[0].IsCancelled() || !items[1].IsCancelled() || !items[2].IsCancelled() {
		t.Fatal("school change flags lost")
	}
}
