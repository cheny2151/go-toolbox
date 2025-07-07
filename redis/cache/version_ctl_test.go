package featurekey

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

type data struct {
	DTOVersion
	V int `json:"v"`
}

type DataService interface {
	FetchData(ctx context.Context, codes []string) ([]*data, error)
}

type DataServiceDemo struct {
}

func (demo *DataServiceDemo) FetchData(ctx context.Context, codes []string) ([]*data, error) {
	return []*data{
		{V: 1, DTOVersion: DTOVersion{Version: 1}},
		{V: 2, DTOVersion: DTOVersion{Version: 2}},
	}, nil
}

type DataServiceCacheWrap struct {
	DataService
	fetchDataHelp CacheHelp[data]
}

func (wrap *DataServiceCacheWrap) FetchData(ctx context.Context, codes []string) ([]*data, error) {
	keyCodeMap := make(map[string]string, len(codes))
	keys := make([]string, len(codes))
	for i, code := range codes {
		key := wrap.getKey(code)
		keys[i] = key
		keyCodeMap[key] = code
	}

	var (
		getKeys        = func() []string { return keys }
		fetchDatesFunc = func(missKeys []string) ([]*data, error) {
			missCodes := make([]string, len(missKeys))
			for i := range missKeys {
				missCodes[i] = keyCodeMap[missKeys[i]]
			}
			return wrap.DataService.FetchData(ctx, missCodes)
		}
	)

	return wrap.fetchDataHelp.DoMultiProxyUseDefaultMarshal(ctx, getKeys, fetchDatesFunc)
}

func (wrap *DataServiceCacheWrap) getKey(code string) string {
	return fmt.Sprintf("test:{%s}", code)
}

func TestCacheWrap(t *testing.T) {
	Init(getRdb())
	demo := DataServiceDemo{}
	wrap := DataServiceCacheWrap{DataService: &demo}
	fetchData, err := wrap.FetchData(context.Background(), []string{"a", "b"})
	if err != nil {
		fmt.Println(err)
		return
	}
	marshal, _ := json.Marshal(fetchData)
	fmt.Println(string(marshal), err)
	time.Sleep(2 * time.Second)
}

func getRdb() redis.UniversalClient {
	rdb := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    "",
		SentinelAddrs: []string{},
		DB:            15,
	})
	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		panic(err)
	}
	return rdb
}
