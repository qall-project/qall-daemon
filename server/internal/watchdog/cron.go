package watchdog

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type Cron struct {
	ctx      context.Context
	callback func() error
	tickTime time.Duration
	stopChan chan bool
	ticker   *time.Ticker
	lock     sync.Mutex
}

func NewCron(ctx context.Context) (*Cron, error) {
	return &Cron{
		ctx:      ctx,
		stopChan: nil,
		ticker:   nil,
	}, nil
}

func (c *Cron) Start(tickTime time.Duration, callback func() error) (err error) {
	c.tickTime = tickTime
	c.callback = callback

	c.stopChan = make(chan bool)
	c.ticker = time.NewTicker(c.tickTime)

	for {
		defer func() {
			if recovery := recover(); recovery != nil {
				err = recovery.(error)
				log.Println("Error during cron's loop:", err)
			}
		}()

		if c.ticker == nil || c.stopChan == nil {
			return
		}

		select {
		case <-c.stopChan:
			return nil
		case <-c.ticker.C:
			c.callback()
		}
	}
}

func (c *Cron) Stop() error {
	if c.ticker == nil || c.stopChan == nil {
		return fmt.Errorf("cron is not running")
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.ticker != nil {
		c.ticker.Stop()
		c.ticker = nil
	}

	if c.stopChan != nil {
		c.stopChan <- true
		close(c.stopChan)
		c.stopChan = nil
	}

	return nil
}
