package usecase

import (
	"strings"
	"time"
)

// BusinessCalendar advances response targets only during configured local
// working hours. When disabled, ticket SLA targets use elapsed hours.
type BusinessCalendar struct {
	Enabled  bool
	Location *time.Location
	Start    time.Duration
	End      time.Duration
	Weekdays map[time.Weekday]bool
	Holidays map[string]bool
}

func NewBusinessCalendar(enabled bool, location *time.Location, start, end, weekdays, holidays string) *BusinessCalendar {
	if location == nil {
		location = time.UTC
	}
	startTime, err := time.Parse("15:04", strings.TrimSpace(start))
	if err != nil {
		startTime, _ = time.Parse("15:04", "09:00")
	}
	endTime, err := time.Parse("15:04", strings.TrimSpace(end))
	if err != nil {
		endTime, _ = time.Parse("15:04", "17:00")
	}
	startMinute := time.Duration(startTime.Hour()*60+startTime.Minute()) * time.Minute
	endMinute := time.Duration(endTime.Hour()*60+endTime.Minute()) * time.Minute
	if endMinute <= startMinute {
		startMinute, endMinute = 9*time.Hour, 17*time.Hour
	}
	workdays := make(map[time.Weekday]bool)
	dayNames := map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}
	for _, part := range strings.Split(strings.ToLower(weekdays), ",") {
		if day, ok := dayNames[strings.TrimSpace(part)]; ok {
			workdays[day] = true
		}
	}
	if len(workdays) == 0 {
		workdays = map[time.Weekday]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}
	}
	holidaySet := make(map[string]bool)
	for _, value := range strings.Split(holidays, ",") {
		value = strings.TrimSpace(value)
		if _, err := time.Parse("2006-01-02", value); err == nil {
			holidaySet[value] = true
		}
	}
	return &BusinessCalendar{Enabled: enabled, Location: location, Start: startMinute, End: endMinute, Weekdays: workdays, Holidays: holidaySet}
}

func (c *BusinessCalendar) DueAt(start time.Time, hours int) time.Time {
	if !c.Enabled || c.Location == nil || len(c.Weekdays) == 0 || hours <= 0 {
		return start.UTC().Add(time.Duration(hours) * time.Hour)
	}
	remaining := time.Duration(hours) * time.Hour
	current := start.In(c.Location)
	for remaining > 0 {
		if !c.isWorkday(current) {
			current = c.nextWorkdayOpen(current)
			continue
		}
		opening := c.openAt(current)
		closing := c.closeAt(current)
		if current.Before(opening) {
			current = opening
		}
		if !current.Before(closing) {
			current = c.nextWorkdayOpen(current)
			continue
		}
		available := closing.Sub(current)
		if remaining <= available {
			return current.Add(remaining).UTC()
		}
		remaining -= available
		current = c.nextWorkdayOpen(current)
	}
	return current.UTC()
}

func (c *BusinessCalendar) isWorkday(value time.Time) bool {
	if !c.Weekdays[value.Weekday()] {
		return false
	}
	return !c.Holidays[value.Format("2006-01-02")]
}

func (c *BusinessCalendar) nextWorkdayOpen(value time.Time) time.Time {
	date := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, c.Location)
	for day := 0; day < 15; day++ {
		date = date.AddDate(0, 0, 1)
		if c.isWorkday(date) {
			return c.openAt(date)
		}
	}
	// A configuration with no workdays in a two-week span should not hang the
	// request path. Use the following day at the configured opening time.
	return c.openAt(date)
}

func (c *BusinessCalendar) openAt(value time.Time) time.Time {
	hour, minute := int(c.Start/time.Hour), int(c.Start%time.Hour/time.Minute)
	return time.Date(value.Year(), value.Month(), value.Day(), hour, minute, 0, 0, c.Location)
}

func (c *BusinessCalendar) closeAt(value time.Time) time.Time {
	hour, minute := int(c.End/time.Hour), int(c.End%time.Hour/time.Minute)
	return time.Date(value.Year(), value.Month(), value.Day(), hour, minute, 0, 0, c.Location)
}
