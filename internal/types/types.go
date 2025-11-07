package types

import (
	"sync"
	"time"
)

type CallStatus string

const (
	StatusActive CallStatus = "active"
	StatusDone   CallStatus = "done"
	StatusError  CallStatus = "error"
)

type Call struct {
	ID        string
	Method    string
	Endpoint  string
	Status    CallStatus
	StartTime time.Time
	EndTime   *time.Time
	Request   string
	Response  string
	mu        sync.Mutex
}

func (c *Call) UpdateResponse(data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Response += data
}

func (c *Call) MarkDone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.EndTime = &now
	c.Status = StatusDone
}

func (c *Call) MarkError() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.EndTime = &now
	c.Status = StatusError
}

type Event struct {
	ID   string
	Data string
	Done bool
}
