package watchdog

import (
	"context"
	"errors"
	"sync"
	"time"
)

type WatchDog interface {
	Watch(string, func() bool, func()) error
	UnWatch(string) error
}

type watched struct {
	Id        string
	Predicate func() bool
	Callback  func()
}

type watchDog struct {
	cron      *Cron
	watchList map[string]watched
	mutex     sync.Mutex
}

func NewWatchdog(ctx context.Context) (*watchDog, error) {
	cron, err := NewCron(ctx)

	if err != nil {
		return nil, err
	}

	twd := &watchDog{
		cron:      cron,
		watchList: make(map[string]watched),
	}

	go cron.Start(3*time.Second, twd.callWatch)

	return twd, nil
}

func (twd *watchDog) Watch(id string, predicate func() bool, callback func()) error {
	if predicate == nil {
		return errors.New("predicate cannot be nil")
	}

	twd.mutex.Lock()
	defer twd.mutex.Unlock()

	_, ok := twd.watchList[id]

	if ok {
		return errors.New("id already exists")
	}

	twd.watchList[id] = watched{
		Id:        id,
		Predicate: predicate,
		Callback:  callback,
	}

	return nil
}

func (twd *watchDog) UnWatch(id string) error {
	twd.mutex.Lock()
	defer twd.mutex.Unlock()

	delete(twd.watchList, id)

	return nil
}

func (twd *watchDog) callWatch() error {
	twd.mutex.Lock()

	watches := make([]watched, 0, len(twd.watchList))

	for _, w := range twd.watchList {
		watches = append(watches, w)
	}

	twd.mutex.Unlock()

	for _, w := range watches {
		if !w.Predicate() {
			continue
		}

		if w.Callback != nil {
			w.Callback()
		}

		twd.mutex.Lock()
		delete(twd.watchList, w.Id)
		twd.mutex.Unlock()
	}

	return nil
}
