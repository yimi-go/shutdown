package posixsignal

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

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

func Test_trigger_Wait(t *testing.T) {
	signals := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM}
	for _, s := range signals {
		t.Run(s.String(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()
			targetTrigger := NewTrigger().(*trigger)
			gs1 := NewMockController(ctrl)
			gs1.EXPECT().
				HandleShutdown(gomock.Any(), gomock.Any()).
				Do(func(c context.Context, event shutdown.Event) {
					assert.Equal(t, fmt.Sprintf("received signal: %s", s), event.Reason())
				}).
				Times(1)
			waiting, wait := make(chan struct{}), make(chan struct{})
			go func() {
				waiting <- struct{}{}
				err := targetTrigger.Wait(ctx, gs1)
				assert.Nil(t, err)
				wait <- struct{}{}
			}()
			<-waiting
			time.Sleep(time.Millisecond)
			_ = syscall.Kill(syscall.Getpid(), s)
			select {
			case <-wait:
			case <-time.After(time.Second):
				t.Error("timeout")
			}
		})
	}
}

func Test_trigger_Wait_cancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	targetTrigger := NewTrigger().(*trigger)
	gs1 := NewMockController(ctrl)
	gs1.EXPECT().HandleShutdown(gomock.Any(), gomock.Any()).
		Do(func(c context.Context, event shutdown.Event) {
			assert.Equal(t, context.Canceled.Error(), event.Reason())
		}).
		Times(1)
	wait := make(chan struct{})
	go func() {
		err := targetTrigger.Wait(ctx, gs1)
		assert.ErrorIs(t, err, context.Canceled)
		wait <- struct{}{}
	}()
	select {
	case <-wait:
	case <-time.After(time.Hour):
		t.Error("timeout")
	}
}
