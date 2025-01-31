package workflow

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cschleiden/go-workflows/internal/args"
	"github.com/cschleiden/go-workflows/internal/converter"
	"github.com/cschleiden/go-workflows/internal/payload"
	"github.com/cschleiden/go-workflows/internal/sync"
)

type Workflow interface{}

type workflow struct {
	s      sync.Scheduler
	fn     reflect.Value
	result payload.Payload
	err    error
}

func NewWorkflow(workflowFn reflect.Value) *workflow {
	s := sync.NewScheduler()

	return &workflow{
		s:  s,
		fn: workflowFn,
	}
}

func (w *workflow) Execute(ctx sync.Context, inputs []payload.Payload) error {
	w.s.NewCoroutine(ctx, func(ctx sync.Context) error {
		args, addContext, err := args.InputsToArgs(converter.DefaultConverter, w.fn, inputs)
		if err != nil {
			return fmt.Errorf("converting workflow inputs: %w", err)
		}

		if !addContext {
			return errors.New("workflow must accept context as first argument")
		}

		args[0] = reflect.ValueOf(ctx)

		// Call workflow function
		r := w.fn.Call(args)

		// Process result
		if len(r) < 1 || len(r) > 2 {
			return errors.New("workflow has to return either (error) or (result, error)")
		}

		var result payload.Payload

		if len(r) > 1 {
			var err error
			result, err = converter.DefaultConverter.To(r[0].Interface())
			if err != nil {
				return fmt.Errorf("converting workflow result: %w", err)
			}
		} else {
			result, err = converter.DefaultConverter.To(nil)
			if err != nil {
				return fmt.Errorf("converting workflow result: %w", err)
			}
		}

		errResult := r[len(r)-1]
		if errResult.IsNil() {
			w.result = result
			return nil
		}

		errInterface, ok := errResult.Interface().(error)
		if !ok {
			return fmt.Errorf("activity error result does not satisfy error interface (%T): %v", errResult, errResult)
		}

		w.err = errInterface

		return nil
	})

	return w.s.Execute(ctx)
}

func (w *workflow) Continue(ctx sync.Context) error {
	return w.s.Execute(ctx)
}

func (w *workflow) Completed() bool {
	return w.s.RunningCoroutines() == 0
}

// Result returns the return value of a finished workflow as a payload
func (w *workflow) Result() payload.Payload {
	return w.result
}

// Error returns the error of a finished workflow, can be nil
func (w *workflow) Error() error {
	return w.err
}

func (w *workflow) Close(ctx sync.Context) {
	// End coroutine execution to prevent goroutine leaks
	w.s.Exit(ctx)
}
