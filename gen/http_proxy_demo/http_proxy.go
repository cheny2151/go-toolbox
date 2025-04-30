/*
这是一个自定义go generate的实例，用来生成http代理，执行http请求
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

//go:generate ./http_proxy_gen -inputFile=./xx.go -outputFile=./xx.go
type HttpProxy[T any] struct {
	httpClient *http.Client
}

func (receiver HttpProxy[T]) Call(ctx context.Context, url string, request any) (*T, error) {
	marshal, _ := json.Marshal(request)
	resp, err := receiver.httpClient.Post(url, "application/json", bytes.NewReader(marshal))
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s请求异常, request:%v\n", url, request)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	t := new(T)
	_ = json.Unmarshal(bodyBytes, t)
	return t, nil
}

// TestProxy 测试代理
// @httpProxy
type TestProxy interface {
	Call(ctx context.Context, test string) (string, error)
}
