package shutdown

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestEventFunc_Reason(t *testing.T) {
	event := EventFunc(func() string {
		return "abc"
	})
	assert.Equal(t, "abc", event.Reason())
}

func TestCallbackFunc_OnShutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	event := NewMockEvent(ctrl)
	shutdownCounter := 0
	testFunc := CallbackFunc(func(_ context.Context, _ Event) error {
		shutdownCounter++
		if shutdownCounter > 1 {
			return errors.New("called more than once")
		}
		return nil
	})
	if err := testFunc.OnShutdown(context.Background(), event); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := testFunc.OnShutdown(context.Background(), event); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestErrorHandleFunc_OnError(t *testing.T) {
	var target error
	param := errors.New("test")
	testFunc := ErrorHandleFunc(func(ctx context.Context, err error) {
		target = err
	})
	testFunc.OnError(context.Background(), param)
	if !reflect.DeepEqual(target, param) {
		t.Errorf("expected %v, got %v", param, target)
	}
}

func TestWithCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	callback := NewMockCallback(ctrl)
	target := &gracefulShutdown{}
	WithCallback(callback)(target)
	for _, cb := range target.callbacks {
		if reflect.DeepEqual(cb, callback) {
			return
		}
	}
	t.Error("target doesn't have the callback")
}

func TestWithTrigger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	trigger := NewMockTrigger(ctrl)
	target := &gracefulShutdown{}
	WithTrigger(trigger)(target)
	for _, tr := range target.triggers {
		if reflect.DeepEqual(tr, trigger) {
			return
		}
	}
	t.Error("target doesn't have the trigger")
}

func TestWithErrorHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	handler := NewMockErrorHandler(ctrl)
	target := &gracefulShutdown{}
	WithErrorHandler(handler)(target)
	if !reflect.DeepEqual(handler, target.errorHandler) {
		t.Errorf("expect %v, got %v", handler, target.errorHandler)
	}
}

func TestWithTimeout(t *testing.T) {
	target := &gracefulShutdown{}
	timeout := time.Duration(rand.Int())
	WithTimeout(timeout)(target)
	if target.timeout != timeout {
		t.Errorf("expected %v, got %v", timeout, target.timeout)
	}
}

func TestNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	callback := NewMockCallback(ctrl)
	trigger1 := NewMockTrigger(ctrl)
	trigger2 := NewMockTrigger(ctrl)
	trigger3 := NewMockTrigger(ctrl)
	trigger4 := NewMockTrigger(ctrl)
	gs := NewGraceful(
		WithCallback(callback),
		WithTrigger(trigger1, trigger2, trigger3, trigger4),
		WithTimeout(time.Second),
	).(*gracefulShutdown)
	if len(gs.callbacks) != 1 {
		t.Errorf("expect gs.callbacks len: 1, got %d", len(gs.callbacks))
	}
	if len(gs.triggers) != 4 {
		t.Errorf("expected gs.triggers len: 4, got %d", len(gs.triggers))
	}
	if gs.timeout != time.Second {
		t.Errorf("expected gs.timeout: %v, got %v", time.Second, gs.timeout)
	}
}

func TestGracefulShutdown_AddTrigger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	gs := &gracefulShutdown{}
	trigger := NewMockTrigger(ctrl)
	gs.AddTrigger(trigger)
	if !reflect.DeepEqual(gs.triggers[len(gs.triggers)-1], trigger) {
		t.Errorf("expect last trigger: %v, got %v", trigger, gs.triggers[len(gs.triggers)-1])
	}
}

func TestGracefulShutdown_Wait(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	trigger := NewMockTrigger(ctrl)
	gs := &gracefulShutdown{}
	ctx := context.Background()
	trigger.EXPECT().Wait(gomock.Any(), gs).Return(nil).Times(15)
	for i := 0; i < 15; i++ {
		gs.AddTrigger(trigger)
	}
	err := gs.Wait(ctx)
	if err != nil {
		t.Errorf("expect no err, got %v", err)
	}
}

func TestGracefulShutdown_Wait_cancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	trigger := NewMockTrigger(ctrl)
	gs := &gracefulShutdown{}
	wg := &sync.WaitGroup{}
	trigger.EXPECT().Wait(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, c Controller) error {
		defer wg.Done()
		<-ctx.Done()
		return nil
	}).Times(15)
	for i := 0; i < 15; i++ {
		wg.Add(1)
		gs.AddTrigger(trigger)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := gs.Wait(ctx)
		if err != nil {
			t.Errorf("expect no err, got %v", err)
		}
	}()
	cancel()
	wg.Wait()
}

func TestGracefulShutdown_Wait_error(t *testing.T) {
	ctrl := gomock.NewController(t)
	gs := &gracefulShutdown{}
	j := rand.Intn(15)
	for i := 0; i < 15; i++ {
		i := i
		trigger := NewMockTrigger(ctrl)
		trigger.EXPECT().Wait(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, Controller) error {
			if i == j {
				return errors.New("test")
			}
			return nil
		})
		gs.AddTrigger(trigger)
	}
	err := gs.Wait(context.Background())
	if err == nil {
		t.Errorf("expect err, got nil")
	}
}

func TestGracefulShutdown_AddShutdownCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	callback := NewMockCallback(ctrl)
	gs := &gracefulShutdown{}
	gs.AddShutdownCallback(callback)
	if !reflect.DeepEqual(gs.callbacks[len(gs.callbacks)-1], callback) {
		t.Errorf("expect last callback: %v, got %v", callback, gs.callbacks[len(gs.callbacks)-1])
	}
}

func TestGracefulShutdown_Shutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	event := NewMockEvent(ctrl)
	callback := NewMockCallback(ctrl)
	gs := NewGraceful(WithCallback(callback))
	callback.EXPECT().OnShutdown(gomock.Any(), event).Times(1)
	gs.HandleShutdown(context.Background(), event)
}

func TestGracefulShutdown_Shutdown_timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	callback := NewMockCallback(ctrl)
	callback.EXPECT().
		OnShutdown(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ Event) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond * 2):
				return errors.New("timeout")
			}
		})
	errHandler := NewMockErrorHandler(ctrl)
	errHandler.EXPECT().OnError(gomock.Any(), gomock.Eq(context.DeadlineExceeded)).Times(1)
	gs := NewGraceful(WithCallback(callback), WithTimeout(time.Millisecond), WithErrorHandler(errHandler))
	gs.HandleShutdown(context.Background(), NewMockEvent(ctrl))
}

func TestGracefulShutdown_ReportError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	callback := NewMockCallback(ctrl)
	callback.EXPECT().OnShutdown(gomock.Any(), gomock.Any()).Return(errors.New("test"))
	errHandler := NewMockErrorHandler(ctrl)
	errHandler.EXPECT().OnError(gomock.Any(), gomock.Any()).Times(1)
	gs := NewGraceful(WithCallback(callback), WithErrorHandler(errHandler))
	gs.HandleShutdown(context.Background(), NewMockEvent(ctrl))
}
