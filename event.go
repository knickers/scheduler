package scheduler

import (
	"errors"
	//"fmt"
	"time"
)

type Event struct {
	index       *int // index into the scheduler's event list
	Action      func() error
	NextTime    time.Time
	RepeatDays  []bool
	RepeatWeeks []bool
}

func (e *Event) UpdateNextTime() error {
	// e.nextTime is thisTime right now
	now := time.Now()
	day := int(e.NextTime.Weekday()) + 1 // weekday = 0-6, start tomorrow (+1)
	_, week := e.NextTime.ISOWeek()      // year, week(1-53)
	week -= 1                            // convert to 0-52
	nextYear := now.AddDate(1, 0, 0)     // Next year for detecting empty events

	for {
		for ; day < 7; day++ {
			// Add one more day to the wait time
			e.NextTime = e.NextTime.AddDate(0, 0, 1)
			// We're done if nextTime is after today and today is enabled
			if e.NextTime.After(now) && e.RepeatWeeks[week] && e.RepeatDays[day] {
				return nil
			}
		}
		// Check to make sure we havn't been looping forever
		if e.NextTime.After(nextYear) {
			break
		}
		week++
		if week > 52 {
			week = 0
		}
		day = 0
	}

	return errors.New("There were no enabled days for this event")
}

func (e *Event) Update(nextTime time.Time, repeatDays, repeatWeeks []bool) error {
	e.NextTime = nextTime
	e.RepeatDays = repeatDays
	e.RepeatWeeks = repeatWeeks
	return nil
}

func (e *Event) Id() int {
	return *e.index
}

func (e *Event) String() string {
	return e.NextTime.Format(Stamp)
}

func NewEvent(t time.Time) *Event {
	e := new(Event)
	e.NextTime = t
	return e
}
