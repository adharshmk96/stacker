package project

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The scheduler is a plain five-field cron, evaluated in UTC — the clock the
// rest of the module stamps its rows with: minute, hour, day-of-month, month,
// day-of-week. Each field takes
// `*`, a number, a `a-b` range, a `*/n` or `a-b/n` step, or a comma-separated
// list of those. Names (`mon`, `jan`) are deliberately not accepted — one
// syntax is easier to explain in the UI hint than two.
//
// Nothing here reaches for a dependency: a cron field is a set of allowed
// numbers, and the whole parser is the code below.

// cronField is the set of values one field allows, indexed from the field's own
// minimum (so minute 0 is bit 0, and month 1 is bit 0 too).
type cronField struct {
	allowed []bool
	// restricted is false for `*`, which is what the day-of-month /
	// day-of-week special case below turns on.
	restricted bool
}

func (f cronField) has(value, min int) bool {
	index := value - min
	return index >= 0 && index < len(f.allowed) && f.allowed[index]
}

// cronSchedule is a parsed expression, ready to be asked about a minute.
type cronSchedule struct {
	minute, hour, dom, month, dow cronField
}

// parseCron reads a five-field expression.
func parseCron(expr string) (cronSchedule, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return cronSchedule{}, fmt.Errorf("%w: expected 5 fields, got %d", ErrCronExpression, len(parts))
	}

	bounds := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	fields := [5]cronField{}
	for i, part := range parts {
		field, err := parseCronField(part, bounds[i][0], bounds[i][1])
		if err != nil {
			return cronSchedule{}, err
		}
		fields[i] = field
	}

	return cronSchedule{
		minute: fields[0], hour: fields[1], dom: fields[2], month: fields[3], dow: fields[4],
	}, nil
}

func parseCronField(spec string, min, max int) (cronField, error) {
	field := cronField{allowed: make([]bool, max-min+1)}
	invalid := func() (cronField, error) {
		return cronField{}, fmt.Errorf("%w: %q is not valid for the %d-%d field", ErrCronExpression, spec, min, max)
	}

	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return invalid()
		}

		step := 1
		if base, stepText, ok := strings.Cut(item, "/"); ok {
			parsed, err := strconv.Atoi(stepText)
			if err != nil || parsed < 1 {
				return invalid()
			}
			step, item = parsed, base
		}

		low, high := min, max
		switch {
		case item == "*":
			field.restricted = field.restricted || step > 1
		default:
			from, to, isRange := strings.Cut(item, "-")
			value, err := strconv.Atoi(strings.TrimSpace(from))
			if err != nil {
				return invalid()
			}
			// Sunday is both 0 and 7 in every cron anyone has used before.
			if max == 6 && value == 7 {
				value = 0
			}
			low, high = value, value
			if isRange {
				high, err = strconv.Atoi(strings.TrimSpace(to))
				if err != nil {
					return invalid()
				}
			}
			field.restricted = true
		}

		if low < min || high > max || low > high {
			return invalid()
		}
		for value := low; value <= high; value += step {
			field.allowed[value-min] = true
		}
	}
	return field, nil
}

// due reports whether the schedule fires in the minute t falls in.
//
// Day-of-month and day-of-week follow the traditional rule: when both are
// restricted the day matches if *either* does, which is what makes
// `0 0 1,15 * mon` mean "the 1st, the 15th, and every Monday".
func (c cronSchedule) due(t time.Time) bool {
	if !c.minute.has(t.Minute(), 0) || !c.hour.has(t.Hour(), 0) || !c.month.has(int(t.Month()), 1) {
		return false
	}

	dom, dow := c.dom.has(t.Day(), 1), c.dow.has(int(t.Weekday()), 0)
	if c.dom.restricted && c.dow.restricted {
		return dom || dow
	}
	return dom && dow
}

// RunScheduler fires the schedule-triggered environments, one check a minute,
// until the context is cancelled.
//
// It wakes on the minute boundary rather than a minute after start, so `0 3 *
// * *` runs at 03:00 whenever the server happened to boot. A minute that is
// missed entirely — the process was down, or a check ran long — is not made up
// afterwards: a deploy of yesterday's 03:00 landing at lunchtime is worse than
// a deploy that did not happen.
func (s *Service) RunScheduler(ctx context.Context) {
	for {
		now := timeNow()
		next := now.Truncate(time.Minute).Add(time.Minute)

		timer := time.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		s.runDue(timeNow())
	}
}

// runDue queues every environment whose cron expression matches this minute.
func (s *Service) runDue(now time.Time) {
	items, err := s.repo.List()
	if err != nil {
		s.log.Error("the scheduler could not read the projects", "error", err)
		return
	}

	for _, item := range items {
		for _, env := range item.Environments {
			if env.Trigger.Kind != TriggerSchedule {
				continue
			}
			schedule, err := parseCron(env.Trigger.Pattern)
			if err != nil {
				// Saving validates the expression, so this is a row written
				// before the rule existed. Log it and leave it alone.
				s.log.Warn("skipping an unparseable schedule",
					"project", item.Name, "environment", env.Name, "pattern", env.Trigger.Pattern)
				continue
			}
			if !schedule.due(now) {
				continue
			}

			_, err = s.engine.Start(item, env, DeployRequest{
				Actor:       "schedule",
				Message:     "scheduled deploy (" + env.Trigger.Pattern + ")",
				TriggeredBy: TriggerSchedule,
			})
			switch {
			case err == nil:
				s.log.Info("scheduled deploy queued", "project", item.Name, "environment", env.Name)
			case errors.Is(err, ErrAlreadyDeploying):
				s.log.Info("skipping a scheduled deploy, one is already running",
					"project", item.Name, "environment", env.Name)
			default:
				s.log.Error("could not start a scheduled deploy",
					"project", item.Name, "environment", env.Name, "error", err)
			}
		}
	}
}
