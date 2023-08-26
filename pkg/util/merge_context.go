package util

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// mergeContext combines multiple contexts.
type mergeContext struct {
	contexts []context.Context
}

func NewMergeContext(contexts []context.Context) context.Context {
	return mergeContext{contexts: contexts}
}

// Deadline returns the earliest Deadline of all contexts.
func (m mergeContext) Deadline() (deadline time.Time, ok bool) {
	for _, ctx := range m.contexts {
		if anotherDeadline, anotherOk := ctx.Deadline(); anotherOk {
			if ok && anotherDeadline.After(deadline) {
				continue
			}
			deadline = anotherDeadline
			ok = anotherOk
		}
	}
	return deadline, ok
}

// Done notifies when the first context is done.
func (m mergeContext) Done() <-chan struct{} {
	ch := make(chan struct{})
	cases := make([]reflect.SelectCase, 0, len(m.contexts))
	for _, ctx := range m.contexts {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})
	}
	go func(cases []reflect.SelectCase, ch chan struct{}) {
		_, _, _ = reflect.Select(cases)
		close(ch)
	}(cases, ch)
	return ch
}

// Err returns the error of any (random) context and nil iff no context has an error.
func (m mergeContext) Err() error {
	for _, ctx := range m.contexts {
		if ctx.Err() != nil {
			return fmt.Errorf("mergeContext wrapped: %w", ctx.Err())
		}
	}
	return nil
}

// Value returns the value for the key if any context has it.
// If multiple contexts have a value for the key, the result is any (random) of them.
func (m mergeContext) Value(key any) any {
	for _, ctx := range m.contexts {
		if value := ctx.Value(key); value != nil {
			return value
		}
	}
	return nil
}
