package project

import (
	"errors"
	"testing"
	"time"
)

func TestParseCronRejectsNonsense(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"5-1 * * * *",
		"*/0 * * * *",
		"a * * * *",
		"1,, * * * *",
		"1-x * * * *",
	} {
		if _, err := parseCron(expr); !errors.Is(err, ErrCronExpression) {
			t.Errorf("parseCron(%q) error = %v, want ErrCronExpression", expr, err)
		}
	}
}

func TestCronDue(t *testing.T) {
	at := func(value string) time.Time {
		t.Helper()
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatalf("parse time: %v", err)
		}
		return parsed
	}

	cases := []struct {
		expr string
		when string
		want bool
	}{
		// 2026-08-18 is a Tuesday (weekday 2).
		{"* * * * *", "2026-08-18T03:07:00Z", true},
		{"0 3 * * *", "2026-08-18T03:00:00Z", true},
		{"0 3 * * *", "2026-08-18T03:01:00Z", false},
		{"0 3 * * *", "2026-08-18T04:00:00Z", false},
		{"*/15 * * * *", "2026-08-18T09:30:00Z", true},
		{"*/15 * * * *", "2026-08-18T09:31:00Z", false},
		{"0 9-17 * * *", "2026-08-18T17:00:00Z", true},
		{"0 9-17 * * *", "2026-08-18T18:00:00Z", false},
		{"30 2 * * tue", "2026-08-18T02:30:00Z", false}, // names are not accepted
		{"30 2 * * 2", "2026-08-18T02:30:00Z", true},
		{"30 2 * * 3", "2026-08-18T02:30:00Z", false},
		// Sunday is both 0 and 7; 2026-08-23 is a Sunday.
		{"0 0 * * 7", "2026-08-23T00:00:00Z", true},
		{"0 0 * * 0", "2026-08-23T00:00:00Z", true},
		// Both day fields restricted means "either", so the 1st matches even
		// though it is not a Tuesday.
		{"0 0 1 * 2", "2026-09-01T00:00:00Z", true},
		{"0 0 1 * 2", "2026-09-08T00:00:00Z", true},
		{"0 0 1 * 2", "2026-09-09T00:00:00Z", false},
		{"0 0 1 1 *", "2026-01-01T00:00:00Z", true},
		{"0 0 1 1 *", "2026-02-01T00:00:00Z", false},
		{"0,30 * * * *", "2026-08-18T11:30:00Z", true},
		{"0,30 * * * *", "2026-08-18T11:15:00Z", false},
	}

	for _, tc := range cases {
		schedule, err := parseCron(tc.expr)
		if err != nil {
			// "tue" is expected to fail parsing, which is the same as never firing.
			if tc.want {
				t.Errorf("parseCron(%q): %v", tc.expr, err)
			}
			continue
		}
		if got := schedule.due(at(tc.when)); got != tc.want {
			t.Errorf("%q due at %s = %v, want %v", tc.expr, tc.when, got, tc.want)
		}
	}
}

func TestRunDueStartsOnlyMatchingEnvironments(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments = []EnvironmentRequest{
		{Name: "nightly", Trigger: DeployTrigger{Kind: TriggerSchedule, Pattern: "0 3 * * *"}, Deploy: DeploySettings{Replicas: 1}},
		{Name: "hourly", Trigger: DeployTrigger{Kind: TriggerSchedule, Pattern: "0 * * * *"}, Deploy: DeploySettings{Replicas: 1}},
		{Name: "manual", Trigger: DeployTrigger{Kind: TriggerManual}, Deploy: DeploySettings{Replicas: 1}},
	}
	if _, err := service.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 04:00 is the hourly environment's minute and nobody else's.
	service.runDue(time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC))

	waitFor(t, "the scheduled deployment to finish", func() bool {
		items, err := service.Deployments("", 10)
		if err != nil || len(items) == 0 {
			return false
		}
		return items[0].Status.Done()
	})

	items, err := service.Deployments("", 10)
	if err != nil {
		t.Fatalf("deployments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("deployments = %d, want only the hourly one: %+v", len(items), items)
	}
	if items[0].Environment != "hourly" || items[0].TriggeredBy != TriggerSchedule || items[0].Actor != "schedule" {
		t.Errorf("deployment = %+v", items[0])
	}
}
