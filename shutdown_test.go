package shutdown

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"
)

type testTrigger struct {
}

func (t *testTrigger) Name() string {
	return "test-trigger"
}

func (t *testTrigger) WaitAsync(_ context.Context, _ Controller) error {
	return nil
}

type testTriggerFunc func(ctx context.Context, c Controller) error

func (t testTriggerFunc) Name() string {
	return "test-trigger-func"
}

func (t testTriggerFunc) WaitAsync(ctx context.Context, c Controller) error {
	return t(ctx, c)
}

type testCallback struct {
	recordTrigger Trigger
}

func (t *testCallback) OnShutdown(_ context.Context, trigger Trigger) error {
	t.recordTrigger = trigger
	return nil
}

type testErrorHandler struct {
	errOnError error
}

func (t *testErrorHandler) OnError(err error) {
	t.errOnError = err
}

func TestCallbackFunc_OnShutdown(t *testing.T) {
	shutdownCounter := 0
	testFunc := CallbackFunc(func(_ context.Context, _ Trigger) error {
		shutdownCounter++
		if shutdownCounter > 1 {
			return errors.New("called more than once")
		}
		return nil
	})
	if err := testFunc.OnShutdown(context.Background(), &testTrigger{}); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if err := testFunc.OnShutdown(context.Background(), &testTrigger{}); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestErrorHandleFunc_OnError(t *testing.T) {
	var target error
	param := errors.New("test")
	testFunc := ErrorHandleFunc(func(err error) {
		target = err
	})
	testFunc.OnError(param)
	if !reflect.DeepEqual(target, param) {
		t.Errorf("expected %v, got %v", param, target)
	}
}

func TestWithCallback(t *testing.T) {
	target := &GracefulShutdown{}
	callback := &testCallback{}
	WithCallback(callback)(target)
	for _, cb := range target.callbacks {
		if reflect.DeepEqual(cb, callback) {
			return
		}
	}
	t.Error("target doesn't have the callback")
}

func TestWithTrigger(t *testing.T) {
	target := &GracefulShutdown{}
	trigger := &testTrigger{}
	WithTrigger(trigger)(target)
	for _, tr := range target.triggers {
		if reflect.DeepEqual(tr, trigger) {
			return
		}
	}
	t.Error("target doesn't have the trigger")
}

func TestWithErrorHandler(t *testing.T) {
	target := &GracefulShutdown{}
	handler := &testErrorHandler{}
	WithErrorHandler(handler)(target)
	if !reflect.DeepEqual(handler, target.errorHandler) {
		t.Errorf("expect %v, got %v", handler, target.errorHandler)
	}
}

func TestWithTimeout(t *testing.T) {
	target := &GracefulShutdown{}
	timeout := time.Duration(rand.Int())
	WithTimeout(timeout)(target)
	if target.timeout != timeout {
		t.Errorf("expected %v, got %v", timeout, target.timeout)
	}
}

func TestNew(t *testing.T) {
	gs := New(
		WithCallback(&testCallback{}),
		WithTrigger(&testTrigger{}, &testTrigger{}, &testTrigger{}, &testTrigger{}),
		WithTimeout(time.Second),
	)
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

func TestGracefulShutdown_SetErrorHandler(t *testing.T) {
	gs := &GracefulShutdown{}
	handler := &testErrorHandler{}
	gs.SetErrorHandler(handler)
	if !reflect.DeepEqual(gs.errorHandler, handler) {
		t.Errorf("expect %v, got %v", handler, gs.errorHandler)
	}
}

func TestGracefulShutdown_AddTrigger(t *testing.T) {
	gs := &GracefulShutdown{}
	trigger := &testTrigger{}
	gs.AddTrigger(trigger)
	if !reflect.DeepEqual(gs.triggers[len(gs.triggers)-1], trigger) {
		t.Errorf("expect last trigger: %v, got %v", trigger, gs.triggers[len(gs.triggers)-1])
	}
}

type testKey struct{}

func TestGracefulShutdown_WaitAsync(t *testing.T) {
	gs := &GracefulShutdown{}
	ch := make(chan struct{}, 100)
	ctx0 := context.WithValue(context.Background(), testKey{}, "bar")
	for i := 0; i < 15; i++ {
		gs.AddTrigger(testTriggerFunc(func(ctx context.Context, c Controller) error {
			if !reflect.DeepEqual(ctx.Value(testKey{}), "bar") {
				t.Errorf("expect foo in ctx: %v, got %v", "bar", ctx.Value("foo"))
			}
			if !reflect.DeepEqual(c, gs) {
				t.Errorf("expect gs, got %v", c)
			}
			ch <- struct{}{}
			return nil
		}))
	}
	err := gs.WaitAsync(ctx0)
	if err != nil {
		t.Errorf("expect no err, got %v", err)
	}
	if len(ch) != 15 {
		t.Errorf("expect called %d times, got %d times", 15, len(ch))
	}
}

func TestGracefulShutdown_WaitAsync_cancel(t *testing.T) {
	gs := &GracefulShutdown{}
	ch := make(chan struct{}, 100)
	wg := &sync.WaitGroup{}
	for i := 0; i < 15; i++ {
		wg.Add(1)
		gs.AddTrigger(testTriggerFunc(func(ctx context.Context, c Controller) error {
			defer wg.Done()
			<-ctx.Done()
			ch <- struct{}{}
			return nil
		}))
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := gs.WaitAsync(ctx)
		if err != nil {
			t.Errorf("expect no err, got %v", err)
		}
	}()
	cancel()
	wg.Wait()
	if len(ch) != 15 {
		t.Errorf("expect called %d times, got %d times", 15, len(ch))
	}
}

func TestGracefulShutdown_WaitAsync_error(t *testing.T) {
	gs := &GracefulShutdown{}
	j := rand.Intn(15)
	for i := 0; i < 15; i++ {
		i := i
		gs.AddTrigger(testTriggerFunc(func(ctx context.Context, c Controller) error {
			if i == j {
				return errors.New("test")
			}
			return nil
		}))
	}
	err := gs.WaitAsync(context.Background())
	if err == nil {
		t.Errorf("expect err, got nil")
	}
}

func TestGracefulShutdown_AddShutdownCallback(t *testing.T) {
	gs := &GracefulShutdown{}
	cb := &testCallback{}
	gs.AddShutdownCallback(cb)
	if !reflect.DeepEqual(gs.callbacks[len(gs.callbacks)-1], cb) {
		t.Errorf("expect last callback: %v, got %v", cb, gs.callbacks[len(gs.callbacks)-1])
	}
}

func TestGracefulShutdown_Shutdown(t *testing.T) {
	callback := &testCallback{}
	gs := New(WithCallback(callback))
	trigger := &testTrigger{}
	gs.Shutdown(trigger)
	if !reflect.DeepEqual(callback.recordTrigger, trigger) {
		t.Errorf("expect trigger %v, got %v", trigger, callback.recordTrigger)
	}
}

func TestGracefulShutdown_Shutdown_timeout(t *testing.T) {
	callback := CallbackFunc(func(ctx context.Context, trigger Trigger) error {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second * 2):
			return errors.New("timeout")
		}
	})
	errHandler := &testErrorHandler{}
	gs := New(WithCallback(callback), WithTimeout(time.Second), WithErrorHandler(errHandler))
	gs.Shutdown(&testTrigger{})
	if errHandler.errOnError != nil {
		t.Errorf("expect no err, got %v", errHandler.errOnError)
	}
}

func TestGracefulShutdown_ReportError(t *testing.T) {
	callback := CallbackFunc(func(ctx context.Context, trigger Trigger) error {
		return errors.New("test")
	})
	errHandler := &testErrorHandler{}
	gs := New(WithCallback(callback), WithErrorHandler(errHandler))
	gs.Shutdown(&testTrigger{})
	if errHandler.errOnError == nil {
		t.Errorf("expect err, got nil")
	}
}
