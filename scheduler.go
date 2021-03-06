package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"msg"
	"os"
	"time"
)

type Scheduler struct {
	Stamp       string // string formatting for printing time
	events      []chan Event
	queueLock   chan []int // index into events list, sorted by next event time
	nextEvent   chan Event
	stopWaiting chan bool
}

func New() *Scheduler {
	s := new(Scheduler)
	s.Stamp = "Jan 2 2006 at 3:04:05PM"
	s.nextEvent = make(chan Event)
	s.queueLock = make(chan []int, 1)
	s.queueLock <- []int{}
	s.stopWaiting = make(chan bool)
	return s
}

func (s *Scheduler) Pop() (Event, error) {
	queue := <-s.queueLock
	if len(queue) == 0 {
		s.queueLock <- queue
		return Event{}, errors.New("The events queue is empty")
	}
	e := <-s.events[queue[0]]
	if len(queue) > 1 {
		queue = queue[1:]
	} else {
		queue = []int{}
	}
	s.queueLock <- queue
	return e, nil
}

func (s *Scheduler) Push(e Event) (int, error) {
	if e.index != nil && *e.index >= 0 && *e.index < len(s.events) {
		s.events[*e.index] <- e
	} else {
		e.index = new(int)
		*e.index = len(s.events)
		evnt := make(chan Event, 1)
		evnt <- e
		s.events = append(s.events, evnt)
	}
	queue := <-s.queueLock
	queue = append(queue, *e.index)
	s.queueLock <- queue
	return *e.index, nil
}

func (s *Scheduler) InsertInOrder(e Event) (int, error) {
	index, err := s.Push(e)
	if err != nil {
		fmt.Println(err)
		return -1, nil
	}
	queue := <-s.queueLock
	for i := len(queue) - 2; i >= 0; i-- {
		evnt := <-s.events[queue[i]]
		nextTime := evnt.NextTime
		s.events[queue[i]] <- evnt
		if e.NextTime.After(nextTime) {
			s.queueLock <- queue
			return index, nil
		}
		tmp := queue[i]
		queue[i] = queue[i+1]
		queue[i+1] = tmp
	}
	s.queueLock <- queue
	return index, nil
}

func (s *Scheduler) GetNextTime() (t time.Time, err error) {
	q := <-s.queueLock

	if len(q) == 0 {
		err = errors.New("No events in the queue")
	} else {
		e := <-s.events[q[0]]
		t = e.NextTime
		s.events[q[0]] <- e
	}

	s.queueLock <- q
	return
}

func (s *Scheduler) feedNextEvent() {
	for {
		nextTime, err := s.GetNextTime()
		if err != nil {
			msg.Logln(" feeder) Didn't find an event. Sleeping.")
			time.Sleep(time.Second)
			continue
		}

		msg.Logf(" feeder) Next event on %v\n", nextTime.Format(s.Stamp))
		timer := time.AfterFunc(nextTime.Sub(time.Now()), func() {
			e, err := s.Pop()
			if err == nil {
				s.nextEvent <- e
				s.stopWaiting <- true
			}
		})

		<-s.stopWaiting
		if timer.Stop() {
			msg.Logln(" feeder) Interrupted while waiting for current event.")
		} else {
			msg.Logln(" feeder) Current event completed.")
		}
	}
}

func (s *Scheduler) ManageEventQueue() {
	go s.feedNextEvent()
	var err error

	// main loop
	for {
		// wait for the next event
		msg.Logln("\nmanager) Waiting for an event...")
		event := <-s.nextEvent
		msg.Logln("manager) Got one an event.")

		// Run the scheduled event
		if event.Action != nil {
			err = event.Action()
			if err != nil {
				fmt.Println(err)
				break
			}
		} else {
			msg.Logln("Event had no action")
		}

		// Update the next time for this event
		err = event.UpdateNextTime()
		if err != nil {
			fmt.Println(err)
			break
		}

		// Put this event back on the queue
		_, err = s.InsertInOrder(event)
		if err != nil {
			fmt.Println(err)
			break
		}
	}
}

// Generate num random events. All events will fire at least once in num * 2
// seconds after starting the program, then they will have random days and weeks
// in the future where they will fire again. These can be used for anything from
// filling in a schedule as a template to testing the output system.
func (s *Scheduler) GenerateRandomEvents(num int, aux func(Event)) {
	for i := 0; i < num; i++ {
		// up to num*2 seconds in the future
		dur := time.Second * time.Duration(rand.Int()%(num*2)+1)
		nextT := time.Now().Add(dur)
		// choose the days of the week to be applied
		var days []bool
		for j := 0; j < 7; j++ {
			r := false
			if rand.Int()%2 == 0 {
				r = true
			}
			days = append(days, r)
		}
		// choose the weeks of the year to be applied
		var weeks []bool
		for j := 0; j < 53; j++ {
			r := false
			if rand.Int()%2 == 0 {
				r = true
			}
			weeks = append(weeks, r)
		}
		e := Event{
			NextTime:    nextT,
			RepeatDays:  days,
			RepeatWeeks: weeks,
		}
		
		index, err := s.InsertInOrder(e)
		if err != nil {
			fmt.Println(err)
			break
		}
		e.index = new(int)
		*e.index = index
		if aux != nil {
			aux(e)
		}
	}
}

// Save the current in-memory schedule to file as a json encoded object
// according to schema.json
func (s *Scheduler) SaveSchedule(file string, aux func(Event)) error {
	var events []Event
	for _, e := range s.events {
		event := <-e
		events = append(events, event)
		e <- event
	}
	//bytes, err := json.Marshal(events)
	bytes, err := json.MarshalIndent(events, "", "\t")
	if err != nil {
		fmt.Println("Marshal:", err)
		return err
	}
	err = ioutil.WriteFile(file, bytes, os.FileMode(0664))
	if err != nil {
		fmt.Println("WriteOut:", err)
		return err
	}
	return nil
}

// Load a schedule file into memory. file should be a json encoded file
// according to schema.json
func (s *Scheduler) LoadSchedule(file string, aux func(Event)) error {
	fp, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Println("ReadFile:", err)
		return err
	}
	//log("%s\n", string(fp))
	var events []Event
	err = json.Unmarshal(fp, &events)
	if err != nil {
		fmt.Println("Unmarshal:", err)
		return err
	}
	// Put all the events in
	for _, e := range events {
		*e.index, err = s.InsertInOrder(e)
		if err != nil {
			fmt.Println("Insert:", err)
			return err
		}
		if aux != nil {
			aux(e)
		}
	}
	return nil
}
