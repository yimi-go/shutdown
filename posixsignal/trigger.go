/*
Package posixsignal provides a listener for a posix signal. By default,
it listens for SIGINT and SIGTERM, but others can be chosen in NewTrigger.
*/
package posixsignal

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/exp/slog"

	"github.com/yimi-go/shutdown"
)

const name = "PosixSignalTrigger"

type trigger struct {
	signals []os.Signal
}

// NewTrigger initializes the trigger.
// As arguments, you can provide os.Signal-s to listen to, if none are given,
// it will default to SIGINT and SIGTERM.
func NewTrigger(sig ...os.Signal) shutdown.Trigger {
	if len(sig) == 0 {
		sig = make([]os.Signal, 2)
		sig[0] = os.Interrupt
		sig[1] = syscall.SIGTERM
	}

	return &trigger{
		signals: sig,
	}
}

// Name returns name of this trigger.
func (t *trigger) Name() string {
	return name
}

// Wait starts listening for posix signals.
func (t *trigger) Wait(ctx context.Context, controller shutdown.Controller) (err error) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, t.signals...)
	// Block until a signal is received, or the Context is Done.
	reason := ""
	select {
	case s := <-c:
		reason = fmt.Sprintf("received signal: %s", s)
	case <-ctx.Done():
		err = ctx.Err()
		reason = err.Error()
	}
	ctx = slog.NewContext(context.Background(), slog.FromContext(ctx))
	controller.HandleShutdown(ctx, shutdown.EventFunc(func() string {
		return reason
	}))
	return
}
