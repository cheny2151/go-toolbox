package common

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type token struct{}

type GoWorks struct {
	workSize  int
	semaphore chan token
}

type GoExecutor[T any] struct {
	*GoWorks
}

func NewGoWorks(workSize int) *GoWorks {
	return &GoWorks{
		workSize:  workSize,
		semaphore: make(chan token, workSize),
	}
}

func CreateExecutor[T any](work *GoWorks) *GoExecutor[T] {
	return &GoExecutor[T]{
		GoWorks: work,
	}
}

type AsyncResult[T any] struct {
	V   *T
	Err error
}

func (executor GoExecutor[T]) Go(ctx context.Context, task func(context.Context) T) (result *T, err error) {
	archan := executor.AsyncGo(ctx, task)

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case rs := <-archan:
		if rs.Err != nil {
			err = rs.Err
		} else {
			result = rs.V
		}
	}
	return
}

func (executor GoExecutor[T]) GoWithTimeout(ctx context.Context, timeout time.Duration, task func(context.Context) T) (result *T, err error) {
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()
	return executor.Go(timeoutCtx, task)
}

func (executor GoExecutor[T]) AsyncGo(ctx context.Context, task func(context.Context) T) <-chan *AsyncResult[T] {
	rschan := make(chan *AsyncResult[T], 1)
	select {
	case executor.semaphore <- token{}:
		var ar AsyncResult[T]
		go func() {
			defer func() {
				if r := recover(); r != nil {
					if err0, ok := r.(error); ok {
						ar.Err = err0
					} else {
						ar.Err = errors.New(fmt.Sprintf("task panic:%v", r))
					}
				}
				<-executor.semaphore
				rschan <- &ar
			}()
			t := task(ctx)
			ar.V = &t
		}()
	case <-ctx.Done():
		rschan <- &AsyncResult[T]{
			Err: errors.New("semaphore acquire timeout"),
		}
	}

	return rschan
}

func (executor GoExecutor[T]) AsyncGoWithTimeout(ctx context.Context, timeout time.Duration, task func(context.Context) T) (<-chan *AsyncResult[T], context.CancelFunc) {
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	return executor.AsyncGo(timeoutCtx, task), cancelFunc
}

type AsyncResultWithIndex[T any] struct {
	AsyncResult[T]
	Index int
}

type GoSliceExecutor[I, O any] struct {
	*GoWorks
}

func CreateSliceExecutor[I, O any](work *GoWorks) *GoSliceExecutor[I, O] {
	return &GoSliceExecutor[I, O]{
		GoWorks: work,
	}
}

func (executor GoSliceExecutor[I, O]) Go(ctx context.Context, inputs []I, task func(context.Context, I) O) (results []O, err error) {
	archan := executor.AsyncGo(ctx, inputs, task)
	results0 := make([]O, len(inputs))

out:
	for i := 0; i < len(inputs); i++ {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break out
		case rs := <-archan:
			if rs.Err != nil {
				err = rs.Err
			} else {
				results0[rs.Index] = *rs.V
			}
		}
	}

	if err == nil {
		results = results0
	}
	return
}

func (executor GoSliceExecutor[I, O]) GoWithTimeout(ctx context.Context, timeout time.Duration, inputs []I, task func(context.Context, I) O) (results []O, err error) {
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()
	return executor.Go(timeoutCtx, inputs, task)
}

func (executor GoSliceExecutor[I, O]) AsyncGo(ctx context.Context, inputs []I, task func(context.Context, I) O) <-chan *AsyncResultWithIndex[O] {
	archan := make(chan *AsyncResultWithIndex[O], len(inputs))
	for i, input := range inputs {
		select {
		case executor.semaphore <- token{}:
			go func(idx int, input *I) {
				ar := AsyncResultWithIndex[O]{
					Index: idx,
				}
				defer func() {
					if r := recover(); r != nil {
						if err0, ok := r.(error); ok {
							ar.Err = err0
						} else {
							ar.Err = errors.New(fmt.Sprintf("task panic:%v", r))
						}
					}
					<-executor.semaphore
					archan <- &ar
				}()
				output := task(ctx, *input)
				ar.V = &output
			}(i, &input)
		case <-ctx.Done():
			archan <- &AsyncResultWithIndex[O]{
				AsyncResult: AsyncResult[O]{
					Err: errors.New("semaphore acquire timeout"),
				},
				Index: i,
			}
		}
	}
	return archan
}

func (executor GoSliceExecutor[I, O]) AsyncGoWithTimeout(ctx context.Context, timeout time.Duration, inputs []I, task func(context.Context, I) O) (<-chan *AsyncResultWithIndex[O], context.CancelFunc) {
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	return executor.AsyncGo(timeoutCtx, inputs, task), cancelFunc
}
