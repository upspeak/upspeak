package scheduler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CronSchedule represents a parsed 5-field cron expression.
// Fields are: minute hour day-of-month month day-of-week.
//
// Day-of-week matching: when both day-of-month and day-of-week are
// restricted (not *), a time matches if EITHER condition is satisfied
// (union semantics). When only one is restricted, match on that one.
type CronSchedule struct {
	minutes  []int // 0-59
	hours    []int // 0-23
	days     []int // 1-31
	months   []int // 1-12
	weekdays []int // 0-6 (Sunday=0, normalised from 0-7 input)
	raw      string
}

// ParseCron parses a standard 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
//
// Supports:
//   - * (any value)
//   - ranges: 1-5
//   - lists: 1,3,5
//   - steps: */15, 1-30/5
//
// Field ranges:
//   - minute: 0-59
//   - hour: 0-23
//   - day-of-month: 1-31
//   - month: 1-12
//   - day-of-week: 0-7 (0 and 7 both represent Sunday)
//
// Returns an error if the expression is malformed or contains
// out-of-range values.
func ParseCron(expr string) (*CronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("cron expression cannot be empty")
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have exactly 5 fields, got %d", len(fields))
	}

	cs := &CronSchedule{raw: expr}
	var err error

	// Parse minute (0-59)
	cs.minutes, err = parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	// Parse hour (0-23)
	cs.hours, err = parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	// Parse day-of-month (1-31)
	cs.days, err = parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}

	// Parse month (1-12)
	cs.months, err = parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	// Parse day-of-week (0-7, normalise 7 to 0 for Sunday)
	cs.weekdays, err = parseField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	// Normalise day-of-week: 7 -> 0 (Sunday)
	for i, wd := range cs.weekdays {
		if wd == 7 {
			cs.weekdays[i] = 0
		}
	}
	// Remove duplicates and sort
	cs.weekdays = uniqueSorted(cs.weekdays)

	return cs, nil
}

// Next returns the next time the cron schedule fires after the given time.
// It searches up to 4 years ahead to find the next match.
// If no match is found within that window, returns the zero time.
func (cs *CronSchedule) Next(from time.Time) time.Time {
	// Start from the next minute (truncate seconds and add 1 minute)
	t := from.Truncate(time.Minute).Add(time.Minute).UTC()

	// Safety cap: search up to 4 years ahead
	maxIterations := 366*4 + 100 // Leap years + buffer
	iterations := 0

	for iterations < maxIterations {
		iterations++

		// Check month
		if !contains(cs.months, int(t.Month())) {
			// Advance to next matching month
			t = cs.nextMonth(t)
			continue
		}

		// Check day (day-of-month OR day-of-week)
		if !cs.matchesDay(t) {
			// Advance to next day at 00:00
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
			continue
		}

		// Check hour
		if !contains(cs.hours, t.Hour()) {
			// Advance to next matching hour
			t = cs.nextHour(t)
			continue
		}

		// Check minute
		if !contains(cs.minutes, t.Minute()) {
			// Advance to next matching minute
			t = cs.nextMinute(t)
			continue
		}

		// All fields match!
		return t
	}

	// No match found within search window
	return time.Time{}
}

// String returns the original cron expression.
func (cs *CronSchedule) String() string {
	return cs.raw
}

// parseField parses a single cron field into a slice of integers.
// Supports: *, N, N-M, */S, N-M/S, and comma-separated lists.
func parseField(field string, min, max int) ([]int, error) {
	if field == "" {
		return nil, fmt.Errorf("field cannot be empty")
	}

	var result []int

	// Handle comma-separated lists
	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		vals, err := parseFieldPart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid values parsed")
	}

	return uniqueSorted(result), nil
}

// parseFieldPart parses a single element: *, N, N-M, */S, or N-M/S.
func parseFieldPart(part string, min, max int) ([]int, error) {
	// Check for step notation
	var step int = 1
	if strings.Contains(part, "/") {
		stepParts := strings.Split(part, "/")
		if len(stepParts) != 2 {
			return nil, fmt.Errorf("invalid step notation: %s", part)
		}
		var err error
		step, err = strconv.Atoi(stepParts[1])
		if err != nil || step < 1 {
			return nil, fmt.Errorf("invalid step value: %s", stepParts[1])
		}
		part = stepParts[0]
	}

	var start, end int

	if part == "*" {
		start = min
		end = max
	} else if strings.Contains(part, "-") {
		// Range notation
		rangeParts := strings.Split(part, "-")
		if len(rangeParts) != 2 {
			return nil, fmt.Errorf("invalid range notation: %s", part)
		}
		var err error
		start, err = strconv.Atoi(rangeParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
		}
		end, err = strconv.Atoi(rangeParts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
		}
		if start > end {
			return nil, fmt.Errorf("range start (%d) must be <= end (%d)", start, end)
		}
	} else {
		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", part)
		}
		start = val
		end = val
	}

	// Validate bounds
	if start < min || start > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", start, min, max)
	}
	if end < min || end > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", end, min, max)
	}

	// Generate values with step
	var result []int
	for i := start; i <= end; i += step {
		result = append(result, i)
	}

	return result, nil
}

// matchesDay checks if the given time matches the day-of-month
// and day-of-week constraints.
//
// Logic:
//   - If both dom and dow are restricted: match if EITHER matches (union)
//   - If only dom is restricted: match dom only
//   - If only dow is restricted: match dow only
//   - If neither is restricted: always match
func (cs *CronSchedule) matchesDay(t time.Time) bool {
	domRestricted := !isFullRange(cs.days, 1, 31)
	dowRestricted := !isFullRange(cs.weekdays, 0, 6)

	if !domRestricted && !dowRestricted {
		// Neither restricted: always match
		return true
	}

	matchesDOM := contains(cs.days, t.Day())
	matchesDOW := contains(cs.weekdays, int(t.Weekday()))

	if domRestricted && dowRestricted {
		// Both restricted: match if EITHER matches (union)
		return matchesDOM || matchesDOW
	}

	if domRestricted {
		return matchesDOM
	}

	return matchesDOW
}

// nextMonth advances time to the first minute of the next matching month.
func (cs *CronSchedule) nextMonth(t time.Time) time.Time {
	currentMonth := int(t.Month())
	nextMonth := findNext(cs.months, currentMonth)

	if nextMonth > currentMonth {
		// Found in same year
		return time.Date(t.Year(), time.Month(nextMonth), 1, 0, 0, 0, 0, time.UTC)
	}

	// Wrap to next year
	return time.Date(t.Year()+1, time.Month(cs.months[0]), 1, 0, 0, 0, 0, time.UTC)
}

// nextHour advances time to the first minute of the next matching hour.
func (cs *CronSchedule) nextHour(t time.Time) time.Time {
	currentHour := t.Hour()
	nextHour := findNext(cs.hours, currentHour)

	if nextHour > currentHour {
		// Found in same day
		return time.Date(t.Year(), t.Month(), t.Day(), nextHour, 0, 0, 0, time.UTC)
	}

	// Wrap to next day
	nextDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), cs.hours[0], 0, 0, 0, time.UTC)
}

// nextMinute advances time to the next matching minute.
func (cs *CronSchedule) nextMinute(t time.Time) time.Time {
	currentMinute := t.Minute()
	nextMinute := findNext(cs.minutes, currentMinute)

	if nextMinute > currentMinute {
		// Found in same hour
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), nextMinute, 0, 0, time.UTC)
	}

	// Wrap to next hour
	nextHour := t.Add(time.Hour)
	return time.Date(nextHour.Year(), nextHour.Month(), nextHour.Day(), nextHour.Hour(), cs.minutes[0], 0, 0, time.UTC)
}

// Utility functions

// contains checks if a slice contains a value.
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// findNext finds the next value in a sorted slice that is greater than current.
// Returns the first element if no greater value exists (for wrapping).
func findNext(sorted []int, current int) int {
	for _, v := range sorted {
		if v > current {
			return v
		}
	}
	return sorted[0] // Wrap around
}

// uniqueSorted returns a sorted slice with duplicates removed.
func uniqueSorted(vals []int) []int {
	if len(vals) == 0 {
		return vals
	}

	sort.Ints(vals)
	result := []int{vals[0]}
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1] {
			result = append(result, vals[i])
		}
	}
	return result
}

// isFullRange checks if a slice contains all values in [min, max].
func isFullRange(vals []int, min, max int) bool {
	if len(vals) != (max - min + 1) {
		return false
	}
	for i := min; i <= max; i++ {
		if !contains(vals, i) {
			return false
		}
	}
	return true
}
