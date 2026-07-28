package main

import (
	"context"
	"errors"
	"sync"
)

var (
	errWebBusy     = errors.New("evaluation busy")
	errWebConflict = errors.New("evaluation ownership conflict")
)

type webRunController struct {
	mu     sync.Mutex
	active *webRunLease
	closed bool
}

type webRunLease struct {
	id       string
	cancel   context.CancelFunc
	enhanced bool
}

func (c *webRunController) admit(parent context.Context, enhanced bool, runID string) (context.Context, *webRunLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.active != nil {
		return nil, nil, errWebBusy
	}
	ctx, cancel := context.WithCancel(parent)
	lease := &webRunLease{cancel: cancel, enhanced: enhanced}
	if enhanced {
		lease.id = runID
	}
	c.active = lease
	return ctx, lease, nil
}

func (c *webRunController) finish(lease *webRunLease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	lease.cancel()
	if c.active == lease {
		c.active = nil
	}
}

func (c *webRunController) cancel(runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if runID == "" || c.active == nil || !c.active.enhanced || c.active.id != runID {
		return errWebConflict
	}
	c.active.cancel()
	return nil
}

func (c *webRunController) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.active != nil {
		c.active.cancel()
	}
}
