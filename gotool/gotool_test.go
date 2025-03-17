package common

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestGoWork_AsyncGo(t *testing.T) {
	ctx := context.Background()
	gw := NewGoWorks(10)
	executor := CreateExecutor[string](gw)
	rss := make([]<-chan *AsyncResult[string], 0)
	ctx, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFunc()
	for i := 0; i < 10; i++ {
		duration := time.Duration(i)
		val := strconv.FormatInt(int64(i), 10)
		future := executor.AsyncGo(ctx, func(ctx0 context.Context) string {
			time.Sleep(duration * time.Second)
			return val
		})
		rss = append(rss, future)
	}
out:
	for _, future := range rss {
		select {
		case <-ctx.Done():
			fmt.Println("timeout")
			break out
		case result := <-future:
			if result.Err == nil {
				fmt.Println(*result.V)
			} else {
				fmt.Println(result.Err)
				break out
			}
		}

	}
	fmt.Println("out")
}

func TestGoWork_SliceGoWithTimeout(t *testing.T) {
	ctx := context.Background()
	gw := NewGoWorks(10)
	executor := CreateSliceExecutor[string, string](gw)
	inputs := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	results, err := executor.GoWithTimeout(ctx, 2*time.Second, inputs, func(ctx context.Context, input string) string {
		v, _ := strconv.ParseInt(input, 10, 64)
		select {
		case <-ctx.Done():
			fmt.Println("input timeout")
			return ""
		case <-time.After(time.Second * time.Duration(v)):
			return input
		}
	})

	if err != nil {
		fmt.Println("err:", err)
	} else {
		for i, result := range results {
			fmt.Println(i, result)
		}
	}

	fmt.Println("out")
}

func TestGoWork_SliceAsyncGoWithTimeout(t *testing.T) {
	ctx := context.Background()
	gw := NewGoWorks(5)
	executor := CreateSliceExecutor[string, string](gw)
	inputs := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	results, cancel := executor.AsyncGoWithTimeout(ctx, 2*time.Second, inputs, func(ctx context.Context, input string) string {
		select {
		case <-ctx.Done():
			panic(errors.New("input timeout"))
		case <-time.After(time.Second):
			return input
		}
	})
	defer cancel()

	for i := 0; i < len(inputs); i++ {
		r := <-results
		if r.Err == nil {
			fmt.Println(*r.V)
		} else {
			fmt.Println(r.Err)
		}
	}

	fmt.Println("out")
}
