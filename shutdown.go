package shutdown

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Event is the shutdown event.
type Event interface {
	// Reason returns the reason of shutting down.
	Reason() string
}

type EventFunc func() string

func (f EventFunc) Reason() string {
	return f()
}

// Callback is an interface you have to implement for shutdown callbacks.
// OnShutdown will be called when shutdown is requested. The parameter
// is the Trigger that requested shutdown.
type Callback interface {
	OnShutdown(ctx context.Context, event Event) error
}

// CallbackFunc is a helper type, so you can easily provide anonymous functions
// as Callbacks.
type CallbackFunc func(ctx context.Context, event Event) error

// OnShutdown defines the action needed to run when shutdown triggered.
func (f CallbackFunc) OnShutdown(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Trigger is an interface implemented by shutdown triggers.
type Trigger interface {
	// Name returns the name of the Trigger.
	Name() string
	// Wait blocks on waiting for a shutdown request.
	Wait(ctx context.Context, c Controller) error
}

// ErrorHandler is an interface you can pass to SetErrorHandler to
// handle asynchronous errors.
type ErrorHandler interface {
	OnError(ctx context.Context, err error)
}

// ErrorHandleFunc is a helper type, so you can easily provide anonymous functions
// as ErrorHandlers.
type ErrorHandleFunc func(ctx context.Context, err error)

// OnError defines the action needed to run when error occurred.
func (f ErrorHandleFunc) OnError(ctx context.Context, err error) {
	f(ctx, err)
}

// Controller is an interface implemented by shutdown controllers,
// that gets passed to Trigger to call Shutdown when shutdown
// is requested.
type Controller interface {
	// AddTrigger adds a Trigger.
	AddTrigger(trigger Trigger)
	// Wait starts all registered triggers waiting for a shutdown request.
	Wait(ctx context.Context) error
	// HandleShutdown starts the shutdown action with event.
	HandleShutdown(ctx context.Context, event Event)
	// AddShutdownCallback adds callback that would be called during shutdown.
	AddShutdownCallback(callback Callback)
}

// gracefulShutdown is main struct that handles Callbacks and
// Triggers. Initialize it with New.
type gracefulShutdown struct {
	callbacks    []Callback
	triggers     []Trigger
	errorHandler ErrorHandler
	timeout      time.Duration
}

// Option is an option func that configs a gracefulShutdown instance.
type Option func(gs *gracefulShutdown)

// WithCallback produces an Option that append Callbacks to a gracefulShutdown.
func WithCallback(callback ...Callback) Option {
	return func(gs *gracefulShutdown) {
		gs.callbacks = append(gs.callbacks, callback...)
	}
}

// WithTrigger produces an Option that append Triggers to a gracefulShutdown.
func WithTrigger(trigger ...Trigger) Option {
	return func(gs *gracefulShutdown) {
		gs.triggers = append(gs.triggers, trigger...)
	}
}

// WithErrorHandler produces an Option that set ErrorHandler for a
// gracefulShutdown.
func WithErrorHandler(errorHandler ErrorHandler) Option {
	return func(gs *gracefulShutdown) {
		gs.errorHandler = errorHandler
	}
}

// WithTimeout produces an Option that set shutdown execution timeout for a
// gracefulShutdown.
func WithTimeout(timeout time.Duration) Option {
	return func(gs *gracefulShutdown) {
		gs.timeout = timeout
	}
}

// NewGraceful initializes a new graceful shutdown controller.
func NewGraceful(options ...Option) Controller {
	gs := &gracefulShutdown{
		callbacks: make([]Callback, 0, 10),
		triggers:  make([]Trigger, 0, 3),
	}
	for _, option := range options {
		option(gs)
	}
	return gs
}

func (gs *gracefulShutdown) AddTrigger(trigger Trigger) {
	gs.triggers = append(gs.triggers, trigger)
}

func (gs *gracefulShutdown) Wait(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, trigger := range gs.triggers {
		trigger := trigger
		eg.Go(func() error {
			return trigger.Wait(ctx, gs)
		})
	}
	return eg.Wait()
}

// AddShutdownCallback adds a Callback that will be called when
// shutdown is requested.
func (gs *gracefulShutdown) AddShutdownCallback(callback Callback) {
	gs.callbacks = append(gs.callbacks, callback)
}

func (gs *gracefulShutdown) HandleShutdown(ctx context.Context, event Event) {
	wg := sync.WaitGroup{}
	if gs.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, gs.timeout)
		defer cancel()
	}
	for _, callback := range gs.callbacks {
		wg.Add(1)
		go func(callback Callback) {
			defer wg.Done()
			gs.onError(ctx, callback.OnShutdown(ctx, event))
		}(callback)
	}
	wg.Wait()
}

func (gs *gracefulShutdown) onError(ctx context.Context, err error) {
	errorHandler := gs.errorHandler
	if err != nil && errorHandler != nil {
		gs.errorHandler.OnError(ctx, err)
	}
}
