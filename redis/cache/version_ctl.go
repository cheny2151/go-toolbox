package featurekey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cheny2151/go-toolbox/slicetool"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
)

const (
	missVer      = 0
	delVer       = -1
	missFal      = "nil"
	mgetCacheLua = `
local rs = {}
for i,hk in ipairs(KEYS) do
	local verk = hk .. '_ver'
	local ver = tonumber(redis.call('get', verk))
	if (ver == nil or ver == false) then
		rs[i] = false
	elseif (ver == -1) then
		rs[i] = 'nil'
	else
		local val = redis.call('hmget', hk, 'v', 'd')
		local curver = tonumber(val[1]) 
		if (curver ~= false and curver ~= nil and curver >= ver) then
			rs[i] = val[2]
		else 
			rs[i] = false
		end
	end
end
return rs
`
	setCacheLua = `
local hk = KEYS[1]
local verk = hk .. '_ver'
	local ver = tonumber(redis.call('get', verk)) or 0
	if (ver == -1) then
		return -1
	end
	if (tonumber(ARGV[1]) >= ver) then
		redis.pcall('set', verk, ARGV[1])
		redis.pcall('hmset', hk, 'v', ARGV[1], 'd', ARGV[2])
		if (tonumber(ARGV[3]) > 0) then
			redis.pcall('expire', hk, ARGV[3])
			redis.pcall('expire', verk, ARGV[3])
		end
		return 0
	end
return ver
`
	setVersionLua = `
local verk = KEYS[1] .. '_ver'
	local ver = tonumber(redis.call('get', verk)) or 0
	local new = tonumber(ARGV[1])
	if (new == -1) then
		redis.call('del', KEYS[1])
		redis.call('set', verk, -1, 'EX', 60)
	elseif (new > ver) then
		redis.call('set', verk, new)
		if (tonumber(ARGV[2]) > 0) then
			redis.call('expire', verk, ARGV[2])
		end
	else return ver
	end
return 0
`

	multiLockLua = `
local locks = {}
local vals = redis.call('mget', unpack(KEYS))
for i,val in ipairs(vals) do
	if (val == false or val == nil) then
		table.insert(locks, KEYS[i])
	end
end
for i,k in ipairs(locks) do
	redis.pcall('set', k, 1, 'EX', ARGV[1])
end
return locks
`
)

var (
	rdb           redis.UniversalClient
	initLuaOnce   sync.Once
	mgetLuaSha1   string
	setLuaSha1    string
	setVerSha1    string
	multiLockSha1 string
)

func Init(rdb0 redis.UniversalClient) {
	rdb = rdb0
}

func initLua() {
	initLuaOnce.Do(func() {
		ctx := context.Background()
		cmd := rdb.ScriptLoad(ctx, mgetCacheLua)
		if cmd.Err() != nil {
			panic(cmd.Err())
		}
		mgetLuaSha1 = cmd.Val()
		cmd = rdb.ScriptLoad(ctx, setCacheLua)
		if cmd.Err() != nil {
			panic(cmd.Err())
		}
		setLuaSha1 = cmd.Val()
		cmd = rdb.ScriptLoad(ctx, setVersionLua)
		if cmd.Err() != nil {
			panic(cmd.Err())
		}
		setVerSha1 = cmd.Val()
		cmd = rdb.ScriptLoad(ctx, multiLockLua)
		if cmd.Err() != nil {
			panic(cmd.Err())
		}
		multiLockSha1 = cmd.Val()
	})
}

type GetDtoVersion interface {
	GetVersion() int
}

type DTOVersion struct {
	Version int `json:"-"`
}

func (v DTOVersion) GetVersion() int {
	return v.Version
}

type CacheHelp[T GetDtoVersion] struct {
	CacheExpire int
	LockTimeout int
	WaitTimeout int
}

type CacheResult[T any] struct {
	Data *T
	OK   bool
}

func (help CacheHelp[T]) DoProxy(ctx context.Context, getKey func() string, marshalCache func(dto *T) ([]byte, error),
	unMarshalCache func(data string) (*T, error), fetchDataFunc func() (*T, error)) (*T, error) {
	key := getKey()

	if cache, ok, err := help.getCache(ctx, key, unMarshalCache); ok {
		return cache, nil
	} else if errors.Is(err, context.Canceled) {
		return nil, err
	}

	var (
		result    *T
		setResult = func(key string, value *T) {
			result = value
		}
	)

	originFunc0 := func(missKey []string) ([]*T, error) {
		data, err := fetchDataFunc()
		if err != nil {
			return nil, err
		}
		return []*T{data}, nil
	}

	err := help.tryLockToExecute(ctx, []string{key}, marshalCache, unMarshalCache, originFunc0, setResult)

	return result, err
}

func (help CacheHelp[T]) DoMultiProxy(ctx context.Context, getKeys func() []string, marshalCache func(dto *T) ([]byte, error),
	unMarshalCache func(data string) (*T, error), fetchDatesFunc func(missKeys []string) ([]*T, error)) ([]*T, error) {

	keys := getKeys()
	keyIndexMap := make(map[string][]int, len(keys))
	for idx, key := range keys {
		if index, ok := keyIndexMap[key]; !ok {
			keyIndexMap[key] = []int{idx}
		} else {
			keyIndexMap[key] = append(index, idx)
		}
	}

	var (
		results   = make([]*T, len(keys))
		missKeys  []string
		setResult = func(key string, result *T) {
			for _, idx := range keyIndexMap[key] {
				results[idx] = result
			}
		}
	)
	caches, err := help.mgetCache(ctx, keys, unMarshalCache)
	if err == nil {
		missKeys = make([]string, 0, len(keys))
		for i, cache := range caches {
			if cache.OK {
				results[i] = cache.Data
			} else {
				missKeys = append(missKeys, keys[i])
			}
		}
	} else if errors.Is(err, context.Canceled) {
		return nil, err
	} else {
		missKeys = keys
	}

	if missKeys != nil && len(missKeys) > 0 {
		err = help.tryLockToExecute(ctx, missKeys, marshalCache, unMarshalCache, fetchDatesFunc, setResult)
	}

	return results, err
}

func (help CacheHelp[T]) DoProxyUseDefaultMarshal(ctx context.Context, getKey func() string, fetchDataFunc func() (*T, error)) (*T, error) {
	return help.DoProxy(ctx, getKey, help.marshalCache, help.unMarshalCache, fetchDataFunc)
}

func (help CacheHelp[T]) DoMultiProxyUseDefaultMarshal(ctx context.Context, getKeys func() []string, fetchDatesFunc func(missKeys []string) ([]*T, error)) ([]*T, error) {
	return help.DoMultiProxy(ctx, getKeys, help.marshalCache, help.unMarshalCache, fetchDatesFunc)
}

// SetNewVersion 设置最新版本号到redis
func (help CacheHelp[T]) SetNewVersion(ctx context.Context, key string, version int) error {
	initLua()
	cmd := rdb.EvalSha(ctx, setVerSha1, []string{key}, version, help.CacheExpire)
	r, err := cmd.Int()
	if err != nil {
		fmt.Printf("设置最新版本失败:%v", err)
		return err
	}
	success := r == 0
	if !success {
		fmt.Printf("设置最新版本结果, key:%s, version:%v, success:%v, newest_version:%v\n", key, version, success, r)
	} else {
		fmt.Printf("设置最新版本结果, key:%s, version:%v, success:%v\n", key, version, success)
	}
	return nil
}

// mgetCache 从缓存中获取多个数据
// unMarshalCache为空则使用默认反序列化CacheProxy.unMarshalCache
func (help CacheHelp[T]) mgetCache(ctx context.Context, keys []string, unMarshalCache func(data string) (*T, error)) ([]*CacheResult[T], error) {
	if unMarshalCache == nil {
		unMarshalCache = help.unMarshalCache
	}

	klen := len(keys)
	if klen == 0 {
		return []*CacheResult[T]{}, nil
	}
	caches, err := help.getCache0(ctx, keys)
	if err != nil {
		fmt.Printf("获取redis缓存异常:%v", err)
		return nil, err
	} else {
		results := make([]*CacheResult[T], klen)
		for i := range caches {
			cache := caches[i]
			result := CacheResult[T]{}
			// 缓存了nil值
			if cache == missFal {
				result.OK = true
			} else if cache != "" {
				data, err := unMarshalCache(cache)
				if err != nil {
					fmt.Printf("获取redis缓存，反序列化异常:%s\n", err)
				} else {
					result.Data = data
					result.OK = true
				}
			}
			results[i] = &result
		}
		return results, nil
	}
}

// getCache 从缓存中获取单个数据
func (help CacheHelp[T]) getCache(ctx context.Context, key string, unMarshalCache func(data string) (*T, error)) (*T, bool, error) {
	cache, err := help.mgetCache(ctx, []string{key}, unMarshalCache)
	if err == nil {
		return cache[0].Data, cache[0].OK, nil
	}
	return nil, false, err
}

func (help CacheHelp[T]) executeAndCache(ctx context.Context, keys []string, marshalCache func(dto *T) ([]byte, error),
	originFunc func([]string) ([]*T, error)) ([]*T, error) {
	if marshalCache == nil {
		marshalCache = help.marshalCache
	}

	dates, err := originFunc(keys)
	if err != nil {
		return nil, err
	}
	var (
		results    = make([]*T, len(keys))
		cacheables = make([]*cacheable, len(keys))
	)
	for i := range dates {
		var (
			version = missVer
			data    = missFal
		)
		result := dates[i]
		results[i] = result
		if result != nil {
			marshal, err := marshalCache(result)
			if err != nil {
				fmt.Printf("缓存redis，json序列化异常:%s\n", err)
			} else {
				version = (*result).GetVersion()
				data = string(marshal)
			}
		}
		cacheables[i] = &cacheable{keys[i], version, data}
	}

	help.asyncSetCache(cacheables)

	return results, nil
}

func (help CacheHelp[T]) tryLockToExecute(ctx context.Context, keys []string,
	marshalCache func(dto *T) ([]byte, error), unMarshalCache func(data string) (*T, error),
	fetchDatesFunc func(missKey []string) ([]*T, error), setResult func(key string, result *T)) error {

	var waitKeys []string
	loadKeys, err := help.tryMultiLocks(ctx, keys)
	if err != nil {
		// 获取锁异常，降级直接读库
		fmt.Printf("获取分布式锁异常, key:%v, err:%v\n", keys, err)
		loadKeys = keys
	}
	if len(loadKeys) > 0 {
		// double check cache
		fmt.Printf("获取分布式锁成功，执行实际查询, key:%v\n", loadKeys)
		caches, err := help.executeAndCache(ctx, loadKeys, marshalCache, fetchDatesFunc)
		if err != nil {
			return err
		}
		for i := range caches {
			key := loadKeys[i]
			cache := caches[i]
			setResult(key, cache)
		}
		waitKeys = slicetool.Subtract(keys, loadKeys)
	} else {
		waitKeys = keys
	}

	// 获取锁失败，尝试从缓存获取直至完成或超时
	if waitKeys != nil && len(waitKeys) > 0 {
		timeout := help.WaitTimeout
		if timeout <= 0 {
			timeout = 10
		}
		ctx0, cancelFunc := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancelFunc()
		fmt.Printf("轮询获取waitKeys缓存:%v", waitKeys)
		for {
			fmt.Printf("轮询获取waitKeys缓存中..., keys:%v", waitKeys)
			caches, err := help.mgetCache(ctx, waitKeys, unMarshalCache)
			if err != nil {
				fmt.Printf("轮询获取缓存异常:%v", err)
			} else {
				nextWaitKeys := make([]string, 0, len(waitKeys))
				for i := range caches {
					cache := caches[i]
					key := waitKeys[i]
					if cache.OK {
						setResult(key, cache.Data)
					} else {
						nextWaitKeys = append(nextWaitKeys, key)
					}
				}
				if len(nextWaitKeys) == 0 {
					fmt.Println("完成轮询获取waitKeys缓存")
					break
				}
				waitKeys = nextWaitKeys
			}
			select {
			case <-ctx0.Done():
				fmt.Println("轮询获取waitKeys缓存超时")
				return ctx0.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	return nil
}

func (help CacheHelp[T]) getCache0(ctx context.Context, keys []string) ([]string, error) {
	initLua()
	cmd := rdb.EvalSha(ctx, mgetLuaSha1, keys)
	results, err := cmd.Slice()
	if err != nil {
		fmt.Printf("获取缓存失败%v", err)
		return nil, err
	}
	if len(keys) != len(results) {
		fmt.Printf("获取缓存失败, 输入输出长度不匹配:%s\n", err)
		return nil, errors.New("fail to get cache")
	}
	values := make([]string, len(keys))
	for i, result := range results {
		if result != nil {
			values[i] = result.(string)
		}
	}
	return values, nil
}

type cacheable struct {
	key     string
	version int
	data    string
}

func (help CacheHelp[T]) asyncSetCache(cacheInfos []*cacheable) {
	go func() {
		ctx := context.Background()
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("异步设置缓存异常:%v", r)
			}
			initLua()
			now := time.Now()
			pipelined, _ := rdb.Pipelined(ctx, func(pipeliner redis.Pipeliner) error {
				for _, info := range cacheInfos {
					pipeliner.EvalSha(ctx, setLuaSha1, []string{info.key}, info.version, info.data, help.CacheExpire)
				}
				return nil
			})
			if pipelined != nil {
				for i, cmder := range pipelined {
					if cmd, ok := cmder.(*redis.Cmd); ok {
						r, err := cmd.Int()
						info := cacheInfos[i]
						if err != nil {
							fmt.Printf("设置缓存失败:%v\n", err)
							continue
						}
						success := r == 0
						if !success {
							fmt.Printf("设置缓存结果, key:%s, version:%v, success:%v, newest_version:%v\n", info.key, info.version, success, r)
						} else {
							fmt.Printf("设置缓存结果, key:%s, version:%v, success:%v\n", info.key, info.version, success)
						}
					}
				}
			}
			fmt.Printf("缓存设置耗时, time:%v, size:%v\n", time.Now().UnixMilli()-now.UnixMilli(), len(cacheInfos))
		}()
	}()
}

func (help CacheHelp[T]) tryMultiLocks(ctx context.Context, keys []string) ([]string, error) {
	initLua()
	locks := make([]string, len(keys))
	for i, key := range keys {
		locks[i] = key + "_lk"
	}
	timeout := help.LockTimeout
	if timeout <= 0 {
		timeout = 1
	}
	cmd := rdb.EvalSha(ctx, multiLockSha1, locks, timeout)
	results, err := cmd.Slice()
	if err != nil {
		fmt.Printf("tryMultiLocks失败:%v", err)
		return nil, err
	}
	successKeys := make([]string, 0, len(keys))
	for _, result := range results {
		if result != nil {
			lock := result.(string)
			successKeys = append(successKeys, lock[:len(lock)-3])
		}
	}
	return successKeys, nil
}

func (help CacheHelp[T]) marshalCache(dto *T) ([]byte, error) {
	marshal, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	return marshal, nil
}

func (help CacheHelp[T]) unMarshalCache(data string) (*T, error) {
	var dto T
	err := json.Unmarshal([]byte(data), &dto)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}
