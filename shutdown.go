package shutdown

import (
	"context"
	"sync"
	"time"
)

// Callback is an interface you have to implement for shutdown callbacks.
// OnShutdown will be called when shutdown is requested. The parameter
// is the Trigger that requested shutdown.
type Callback interface {
	OnShutdown(ctx context.Context, trigger Trigger) error
}

// CallbackFunc is a helper type, so you can easily provide anonymous functions
// as Callbacks.
type CallbackFunc func(ctx context.Context, trigger Trigger) error

// OnShutdown defines the action needed to run when shutdown triggered.
func (f CallbackFunc) OnShutdown(ctx context.Context, trigger Trigger) error {
	return f(ctx, trigger)
}

// Trigger is an interface implemented by shutdown triggers.
type Trigger interface {
	// Name returns the name of the Trigger.
	Name() string
	// WaitAsync starts a goroutine waiting for a shutdown request.
	WaitAsync(ctx context.Context, c Controller) error
}

// ErrorHandler is an interface you can pass to SetErrorHandler to
// handle asynchronous errors.
type ErrorHandler interface {
	OnError(err error)
}

// ErrorHandleFunc is a helper type, so you can easily provide anonymous functions
// as ErrorHandlers.
type ErrorHandleFunc func(err error)

// OnError defines the action needed to run when error occurred.
func (f ErrorHandleFunc) OnError(err error) {
	f(err)
}

// Controller is an interface implemented by shutdown controllers,
// that gets passed to Trigger to call Shutdown when shutdown
// is requested.
type Controller interface {
	// WaitAsync starts a new goroutine waiting the Triggers to be triggerred
	// by a shutdown request.
	WaitAsync(ctx context.Context) error
	// Shutdown starts the shutdown action requested by the Trigger.
	Shutdown(trigger Trigger)
	// ReportError handles errors during shutdown.
	ReportError(err error)
	// AddShutdownCallback adds callback that would be called during shutdown.
	AddShutdownCallback(callback Callback)
}

// GracefulShutdown is main struct that handles Callbacks and
// Triggers. Initialize it with New.
type GracefulShutdown struct {
	callbacks    []Callback
	triggers     []Trigger
	errorHandler ErrorHandler
	timeout      time.Duration
}

// Option is an option func that configs a GracefulShutdown instance.
type Option func(gs *GracefulShutdown)

// WithCallback produces an Option that append Callbacks to a GracefulShutdown.
func WithCallback(callback ...Callback) Option {
	return func(gs *GracefulShutdown) {
		gs.callbacks = append(gs.callbacks, callback...)
	}
}

// WithTrigger produces an Option that append Triggers to a GracefulShutdown.
func WithTrigger(trigger ...Trigger) Option {
	return func(gs *GracefulShutdown) {
		gs.triggers = append(gs.triggers, trigger...)
	}
}

// WithErrorHandler produces an Option that set ErrorHandler for a
// GracefulShutdown.
func WithErrorHandler(errorHandler ErrorHandler) Option {
	return func(gs *GracefulShutdown) {
		gs.errorHandler = errorHandler
	}
}

// WithTimeout produces an Option that set shutdown execution timeout for a
// GracefulShutdown.
func WithTimeout(timeout time.Duration) Option {
	return func(gs *GracefulShutdown) {
		gs.timeout = timeout
	}
}

// New initializes a new GracefulShutdown.
func New(options ...Option) *GracefulShutdown {
	gs := &GracefulShutdown{
		callbacks: make([]Callback, 0, 10),
		triggers:  make([]Trigger, 0, 3),
	}
	for _, option := range options {
		option(gs)
	}
	return gs
}

// SetErrorHandler sets an ErrorHandler that will be called when an error
// is encountered in Callback or in Trigger.
func (gs *GracefulShutdown) SetErrorHandler(errorHandler ErrorHandler) {
	gs.errorHandler = errorHandler
}

// AddTrigger adds a Trigger that will wait for shutdown requests.
func (gs *GracefulShutdown) AddTrigger(trigger Trigger) {
	gs.triggers = append(gs.triggers, trigger)
}

// WaitAsync calls Trigger.WaitAsync on all added Triggers. The Triggers start to wait for
// shutdown requests. Returns an error if any Trigger.WaitAsync return an error.
func (gs *GracefulShutdown) WaitAsync(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, trigger := range gs.triggers {
		if err := trigger.WaitAsync(ctx, gs); err != nil {
			return err
		}
	}
	return nil
}

// AddShutdownCallback adds a Callback that will be called when
// shutdown is requested.
func (gs *GracefulShutdown) AddShutdownCallback(callback Callback) {
	gs.callbacks = append(gs.callbacks, callback)
}

func (gs *GracefulShutdown) Shutdown(trigger Trigger) {
	wg := sync.WaitGroup{}
	ctx := context.Background()
	if gs.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, gs.timeout)
		defer cancel()
	}
	for _, callback := range gs.callbacks {
		wg.Add(1)
		go func(callback Callback) {
			defer wg.Done()
			gs.ReportError(callback.OnShutdown(ctx, trigger))
		}(callback)
	}
	wg.Wait()
}

// ReportError is a function that can be used to report errors to ErrorHandler.
func (gs *GracefulShutdown) ReportError(err error) {
	errorHandler := gs.errorHandler
	if err != nil && errorHandler != nil {
		gs.errorHandler.OnError(err)
	}
}
