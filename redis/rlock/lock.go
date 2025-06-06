package rlock

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

const (
	keyPre        = "TOOLBOX"
	lockChannel   = "TOOLBOX:LOCK_CHANNEL:"
	lockIdKey     = "lock_id"
	useLeaseKey   = "use_lease"
	countdownFlag = 0
	unlockFlag    = 1
)

type RLock interface {
	TryLock(ctx context.Context, waitTime, leaseTime time.Duration) (context.Context, bool, error)
	Unlock(ctx context.Context) error
}

type LockSupport interface {
	buildPath(base, hash string) string
	eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
	expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	subUnlock(channel string)
	addListener(key string, lst *unlockListener)
	tryLock0(ctx context.Context, waitTime, leaseTime time.Duration, argsFunc lockArgs) (context.Context, bool, error)
	unlock0(ctx context.Context, argsFunc unlockArgs) error
	close()
}

type unlockListener struct {
	action  func(channel, msg string)
	timeout bool
}

type lockArgs func(waitTime, leaseTime time.Duration, lockId string) (path, lua string, keys []string, argv []any)
type unlockArgs func(lockId string) (path, lua string, keys []string, argv []any)

type UnlockListeners struct {
	listeners []*unlockListener
}

type UnlockListenerAction struct {
	key    string
	lst    *unlockListener
	action byte
}

type BaseLock struct {
	rdb             redis.UniversalClient
	unlockListeners map[string]*UnlockListeners
	closed          bool
	actionChan      chan UnlockListenerAction
	leaseHolder     *LeaseHolder
}

func (lk *BaseLock) initListener() {
	lk.actionChan = make(chan UnlockListenerAction)
	lk.unlockListeners = make(map[string]*UnlockListeners)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Default().Printf("recover lock listener, err:%v", r)
			}
		}()
		for lk.closed == false {
			action := <-lk.actionChan
			key := action.key
			switch action.action {
			case 0:
				delete(lk.unlockListeners, key)
			case 1:
				if listeners, ok := lk.unlockListeners[key]; ok {
					listeners.listeners = append(listeners.listeners, action.lst)
					for i, listener := range listeners.listeners {
						if listener != nil && listener.timeout {
							listeners.listeners[i] = nil
						}
					}
				} else {
					listenerArr := make([]*unlockListener, 1)
					listenerArr[0] = action.lst
					lk.unlockListeners[key] = &UnlockListeners{listenerArr}
				}
			}
		}
	}()
}

func (lk *BaseLock) buildPath(base, hash string) string {
	return fmt.Sprintf("%s:%s:{%s}", keyPre, base, hash)
}

func (lk *BaseLock) eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return lk.rdb.Eval(ctx, script, keys, args)
}

func (lk *BaseLock) expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd {
	return lk.rdb.Expire(ctx, key, expiration)
}

func (lk *BaseLock) subUnlock(channel string) {
	sub := lk.rdb.PSubscribe(context.TODO(), channel)
	go func() {
		defer sub.Close()
		unlockListeners := lk.unlockListeners
		for lk.closed == false {
			msg, err := sub.ReceiveMessage(context.TODO())
			if err != nil {
				continue
			}
			key := msg.Payload
			if listeners, ok := unlockListeners[key]; ok {
				// 为了保证listener不丢失：actionChan设置为无缓冲，确保消费了action: 0的操作（即删除map中的key）后才消费listeners
				lk.actionChan <- UnlockListenerAction{key: key, lst: nil, action: 0}
				for _, listener := range listeners.listeners {
					if listener != nil {
						listener.action(channel, key)
					}
				}
			}
		}
	}()
}

func (lk *BaseLock) addListener(key string, lst *unlockListener) {
	lk.actionChan <- UnlockListenerAction{key: key, lst: lst, action: 1}
}

// tryLock0 通用加锁逻辑，从lockArgs函数中获取加锁脚本所需参数
// 使用chan进行阻塞等待，收到redis解锁广播或者超时后将继续唤醒
func (lk *BaseLock) tryLock0(ctx context.Context, waitTime, leaseTime time.Duration, argsFunc lockArgs) (context.Context, bool, error) {
	if lk.closed {
		return ctx, false, errors.New("lock system has been closed")
	}
	endTime := time.Now().UnixMilli() + waitTime.Milliseconds()
	lockId, ctx := labelCtx(ctx)

	firstLeaseTime := leaseTime
	useLease := false
	if leaseTime < 0 || leaseTime.Milliseconds() > leaseThreshold {
		useLease = true
		firstLeaseTime = time.Millisecond * time.Duration(duration)
	}

	path, lockLua, keys, argv := argsFunc(waitTime, firstLeaseTime, lockId)
	locked := false
	for true {
		_, err := lk.eval(ctx, lockLua, keys, argv...).Result()
		if err == redis.Nil {
			locked = true
			break
		} else if err != nil {
			return ctx, false, err
		}
		nowMilli := time.Now().UnixMilli()
		if endTime >= nowMilli {
			done := make(chan struct{}, 1)
			dur := time.Millisecond * time.Duration(endTime-nowMilli)
			listener := unlockListener{
				action: func(channel, msg string) {
					done <- struct{}{}
					close(done)
				},
			}
			lk.addListener(path, &listener)
			select {
			case <-time.After(dur):
				listener.timeout = true
			case <-done:
			}
		} else {
			break
		}
	}

	if locked && useLease {
		lk.leaseHolder.addLease(path, time.Now().UnixMilli()+leaseTime.Milliseconds())
	}

	ctx = context.WithValue(ctx, useLeaseKey, useLease)

	return ctx, locked, nil
}

// unlock0 通用解锁逻辑，从unlockArgs函数中获取解锁脚本所需参数
func (lk *BaseLock) unlock0(ctx context.Context, argsFunc unlockArgs) error {
	value := ctx.Value(lockIdKey)
	if value == nil {
		return errors.New("can not find lockId in context, please use TryLock returned context")
	}
	lockId := value.(string)
	path, unlockLua, keys, argv := argsFunc(lockId)
	result, err := lk.eval(ctx, unlockLua, keys, argv...).Result()
	if err != nil {
		return err
	}
	if err == redis.Nil {
		// 锁存在但当前ctx未持有该锁
		return nil
	}
	val := result.(int64)
	if val == countdownFlag {
		// countdown
	} else if val == unlockFlag {
		useLease := ctx.Value(useLeaseKey)
		if useLease != nil && useLease.(bool) {
			lk.leaseHolder.removeLease(path)
		}
	}
	return nil
}

func (lk *BaseLock) close() {
	lk.closed = true
}

type LockFactory interface {
	NewReentrantLock(path string) *ReentrantLock
	Close()
}

type LockCreator struct {
	rdb         redis.UniversalClient
	lockSupport LockSupport
}

func NewLockFactory(rdb redis.UniversalClient) LockFactory {
	baseLock := &BaseLock{
		rdb:    rdb,
		closed: false,
	}
	leaseHolder := newLeaseHolder(baseLock)
	baseLock.leaseHolder = leaseHolder
	baseLock.initListener()
	baseLock.subUnlock(lockChannel + "*")
	return &LockCreator{rdb: rdb, lockSupport: baseLock}
}

func (lf *LockCreator) NewReentrantLock(key string) *ReentrantLock {
	baseLock := lf.lockSupport
	path := baseLock.buildPath(reentrantPrefixKey, key)
	return &ReentrantLock{baseLock, key, path}
}

func (lf *LockCreator) Close() {
	lf.lockSupport.close()
}

func labelCtx(ctx context.Context) (string, context.Context) {
	value := ctx.Value(lockIdKey)

	if value != nil {
		lockId := value.(string)
		return lockId, ctx
	} else {
		lockId := uuid.New().String()
		ctx = context.WithValue(ctx, lockIdKey, lockId)
		return lockId, ctx
	}
}
