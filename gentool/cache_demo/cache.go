package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

var rdb redis.UniversalClient

func init() {
	rdb = redis.NewUniversalClient(&redis.UniversalOptions{})
}

type Entity struct {
	A string
}

// TestCacheInterface @cache
type TestCacheInterface interface {
	// DoSomething @cacheMethod
	// @key: ai_test:${str}
	// @expiration: 10*time.Second
	DoSomething(ctx context.Context, str string) (string, error)
	DoSomethingNoCache(ctx context.Context, str string) string
	// DoSomething2 @cacheMethod
	// @key: ai_test2:${business}_${str}
	// @expiration: 2*time.Second
	DoSomething2(ctx context.Context, business int, str string) (*Entity, error)
	DoSomething3(ctx context.Context, business int, str []string) (*Entity, error)
}

type TestCache struct {
}

func (t TestCache) DoSomething(ctx context.Context, str string) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (t TestCache) DoSomethingNoCache(ctx context.Context, str string) string {
	//TODO implement me
	panic("implement me")
}

type TestCacheWrap struct {
	origin TestCacheInterface
}

func (t TestCacheWrap) DoSomething2(ctx context.Context, business int, str string) (*Entity, error) {
	//TODO implement me
	panic("implement me")
}

func (t TestCacheWrap) DoSomething3(ctx context.Context, business int, str []string) (*Entity, error) {

	//TODO implement me
	panic("implement me")
}

func (t TestCacheWrap) DoSomething(ctx context.Context, str string) (string, error) {
	key := fmt.Sprintf("at_test:%s", str)
	resp := rdb.Get(ctx, key)
	err := resp.Err()
	if !errors.Is(err, redis.Nil) {

	} else if err == nil {
		var result string
		bytes, _ := resp.Bytes()
		err := json.Unmarshal(bytes, &result)
		if err != nil {
			return "", err
		} else {
			return result, nil
		}
	}
	result, _ := t.origin.DoSomething(ctx, str)
	if result != "" {
		marshal, err := json.Marshal(result)
		if err != nil {
			return "", err
		} else {
			rdb.Set(ctx, key, string(marshal), time.Second)
		}
	}
	return result, nil
}

func (t TestCacheWrap) DoSomethingNoCache(ctx context.Context, str string) string {
	return t.origin.DoSomethingNoCache(ctx, str)
}
