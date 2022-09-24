package posixsignal

import (
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/yimi-go/shutdown"
)

func TestNewTrigger(t *testing.T) {
	type args struct {
		sig []os.Signal
	}
	tests := []struct {
		name string
		args args
		want *trigger
	}{
		{
			name: "default",
			args: args{sig: []os.Signal{}},
			want: &trigger{signals: []os.Signal{os.Interrupt, syscall.SIGTERM}},
		},
		{
			name: "interrupt",
			args: args{sig: []os.Signal{syscall.SIGINT}},
			want: &trigger{signals: []os.Signal{syscall.SIGINT}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewTrigger(tt.args.sig...).(*trigger); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewTrigger() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_trigger_Name(t *testing.T) {
	target := &trigger{}
	if name != target.Name() {
		t.Errorf("expected %v, got %v", name, target.Name())
	}
}

type shutdownFunc func(trigger shutdown.Trigger)

func (s shutdownFunc) WaitAsync(context.Context) error       { return nil }
func (s shutdownFunc) Shutdown(trigger shutdown.Trigger)     { s(trigger) }
func (s shutdownFunc) ReportError(error)                     {}
func (s shutdownFunc) AddShutdownCallback(shutdown.Callback) {}

func waitSig(t *testing.T, c <-chan struct{}) {
	select {
	case <-c:
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

func Test_trigger_WaitAsync(t *testing.T) {
	ch := make(chan struct{}, 100)

	targetTrigger := NewTrigger().(*trigger)

	_ = targetTrigger.WaitAsync(context.Background(), shutdownFunc(func(trigger shutdown.Trigger) {
		ch <- struct{}{}
	}))
	time.Sleep(time.Millisecond)
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	waitSig(t, ch)

	_ = targetTrigger.WaitAsync(context.Background(), shutdownFunc(func(trigger shutdown.Trigger) {
		ch <- struct{}{}
	}))
	time.Sleep(time.Millisecond)
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	waitSig(t, ch)

	targetTrigger = NewTrigger(syscall.SIGHUP).(*trigger)

	_ = targetTrigger.WaitAsync(context.Background(), shutdownFunc(func(trigger shutdown.Trigger) {
		ch <- struct{}{}
	}))
	time.Sleep(time.Millisecond)
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
	waitSig(t, ch)
}

func Test_trigger_WaitAsync_cancel(t *testing.T) {
	ch := make(chan struct{}, 100)

	targetTrigger := NewTrigger().(*trigger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = targetTrigger.WaitAsync(ctx, shutdownFunc(func(trigger shutdown.Trigger) {
		ch <- struct{}{}
	}))
	waitSig(t, ch)
}
