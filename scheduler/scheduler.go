package scheduler

import (
	"fmt"
	"math"
	"time"
)

// Scheduler defines the interface to control the rate of request.
type Scheduler interface {
	Next(currentReq int64) (wait time.Duration, stop bool)
}

type CountLimiter struct {
	s     Scheduler
	limit int64
}

func NewCountLimiter(s Scheduler, limit int64) CountLimiter {
	return CountLimiter{s, limit}
}

func (cl CountLimiter) Next(currentReq int64) (time.Duration, bool) {
	if currentReq >= cl.limit {
		return 0, false
	}
	return cl.s.Next(currentReq)
}

// A Constant defines a constant rate of requests.
type Constant struct {
	interval time.Duration
}

func NewConstant(freq uint64) (Constant, error) {
	if freq == 0 {
		return Constant{}, fmt.Errorf("freq must be positive")
	}
	return Constant{time.Second / time.Duration(freq)}, nil
}

// Next determines the length of time to sleep until the next request is sent.
func (cp Constant) Next(currentReq int64) (time.Duration, bool) {
	return time.Duration(currentReq) * cp.interval, true
}

// Unlimited defines a unlimited rate of request.
type Unlimited struct{}

// Next determines the length of time to sleep until the next request is sent.
func (cp Unlimited) Next(_ int64) (time.Duration, bool) {
	return 0, true
}

// Line defines a line rate of request.
type Line struct {
	b          float64
	twoA       float64
	bSquare    float64
	bilionDivA float64
}

func NewLine(from, to float64, d time.Duration) Line {
	a := (to - from) / float64(d/1e9)
	b := from
	return Line{
		b:          from,
		twoA:       2 * a,
		bSquare:    b * b,
		bilionDivA: 1e9 / a,
	}
}

// Next determines the length of time to sleep until the next request is sent.
func (cp Line) Next(currentReq int64) (time.Duration, bool) {
	return time.Duration((math.Sqrt(cp.twoA*float64(currentReq)+cp.bSquare) - cp.b) * cp.bilionDivA), true
}
