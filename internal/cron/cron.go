package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidCron = errors.New("invalid cron expression format")
)

// Schedule represents a parsed 5-field cron schedule: minute, hour, dom, month, dow.
type Schedule struct {
	minutes map[int]bool
	hours   map[int]bool
	doms    map[int]bool
	months  map[int]bool
	dows    map[int]bool
}

// Parse parses a standard 5-field cron expression (e.g. "*/5 * * * *").
func Parse(spec string) (*Schedule, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("%w: expected 5 fields, got %d", ErrInvalidCron, len(fields))
	}

	min, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}

	hr, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}

	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}

	mo, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}

	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return &Schedule{
		minutes: min,
		hours:   hr,
		doms:    dom,
		months:  mo,
		dows:    dow,
	}, nil
}

func parseField(field string, min, max int) (map[int]bool, error) {
	res := make(map[int]bool)

	if field == "*" {
		for i := min; i <= max; i++ {
			res[i] = true
		}
		return res, nil
	}

	parts := strings.Split(field, ",")
	for _, part := range parts {
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, ErrInvalidCron
			}
			for i := min; i <= max; i += step {
				res[i] = true
			}
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, ErrInvalidCron
			}
			start, err1 := strconv.Atoi(rangeParts[0])
			end, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil || start > end || start < min || end > max {
				return nil, ErrInvalidCron
			}
			for i := start; i <= end; i++ {
				res[i] = true
			}
			continue
		}

		val, err := strconv.Atoi(part)
		if err != nil || val < min || val > max {
			return nil, ErrInvalidCron
		}
		res[val] = true
	}

	return res, nil
}

// Next calculates the next execution time after t according to the schedule.
func (s *Schedule) Next(t time.Time) time.Time {
	// Advance by 1 minute, zeroing seconds and nanoseconds
	next := t.Truncate(time.Minute).Add(time.Minute)

	// Search up to 5 years into the future
	limit := next.AddDate(5, 0, 0)

	for next.Before(limit) {
		if !s.months[int(next.Month())] {
			next = time.Date(next.Year(), next.Month()+1, 1, 0, 0, 0, 0, next.Location())
			continue
		}

		if !s.doms[next.Day()] || !s.dows[int(next.Weekday())] {
			next = time.Date(next.Year(), next.Month(), next.Day()+1, 0, 0, 0, 0, next.Location())
			continue
		}

		if !s.hours[next.Hour()] {
			next = time.Date(next.Year(), next.Month(), next.Day(), next.Hour()+1, 0, 0, 0, next.Location())
			continue
		}

		if !s.minutes[next.Minute()] {
			next = next.Add(time.Minute)
			continue
		}

		return next
	}

	return time.Time{}
}
