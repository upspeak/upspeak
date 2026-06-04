package scheduler

import (
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every minute", "* * * * *", false},
		{"every 15 minutes", "*/15 * * * *", false},
		{"specific time", "30 2 * * *", false},
		{"weekdays at 9am", "0 9 * * 1-5", false},
		{"monthly first day", "0 0 1 * *", false},
		{"complex", "0,30 9-17 * * 1-5", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"invalid minute", "60 * * * *", true},
		{"invalid hour", "* 25 * * *", true},
		{"invalid day", "* * 32 * *", true},
		{"invalid month", "* * * 13 *", true},
		{"invalid dow", "* * * * 8", true},
		{"invalid range", "5-3 * * * *", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCron(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestCronSchedule_Next(t *testing.T) {
	base := time.Date(2026, 6, 4, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		expr string
		from time.Time
		want time.Time
	}{
		{
			"every minute advances to next",
			"* * * * *",
			base,
			time.Date(2026, 6, 4, 15, 31, 0, 0, time.UTC),
		},
		{
			"every 15 minutes",
			"*/15 * * * *",
			base,
			time.Date(2026, 6, 4, 15, 45, 0, 0, time.UTC),
		},
		{
			"specific time tomorrow",
			"0 9 * * *",
			time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
		},
		{
			"next month first day",
			"0 0 1 * *",
			time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"next Monday (dow=1)",
			"0 9 * * 1",
			time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC), // Thursday
			time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC),  // Monday
		},
		{
			"year wrap",
			"0 0 1 1 *",
			time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, err := ParseCron(tt.expr)
			if err != nil {
				t.Fatalf("ParseCron(%q) error: %v", tt.expr, err)
			}
			got := cs.Next(tt.from)
			if !got.Equal(tt.want) {
				t.Errorf("Next(%v) = %v, want %v", tt.from, got, tt.want)
			}
		})
	}
}
