package usecase

import (
	"testing"
	"time"
)

func TestBusinessCalendarSkipsWeekendsAndConfiguredHolidays(t *testing.T) {
	location, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Fatal(err)
	}
	calendar := NewBusinessCalendar(true, location, "09:00", "17:00", "Mon,Tue,Wed,Thu,Fri", "2024-01-08")
	start := time.Date(2024, time.January, 5, 16, 0, 0, 0, location) // Friday, one work hour remains.
	got := calendar.DueAt(start, 4)
	want := time.Date(2024, time.January, 9, 12, 0, 0, 0, location) // Monday is a holiday; Tuesday supplies three hours.
	if !got.Equal(want) {
		t.Fatalf("DueAt() = %s, want %s", got.In(location), want)
	}
}

func TestBusinessCalendarKeepsElapsedHoursWhenDisabled(t *testing.T) {
	location, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Fatal(err)
	}
	calendar := NewBusinessCalendar(false, location, "09:00", "17:00", "Mon,Tue,Wed,Thu,Fri", "2024-01-08")
	start := time.Date(2024, time.January, 5, 16, 0, 0, 0, location)
	got := calendar.DueAt(start, 4)
	want := start.Add(4 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("DueAt() = %s, want elapsed target %s", got, want)
	}
}
