package rlock

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// permanent 永久续租过期时间
	permanent = int64(-1)
	// duration 一次续租的时长:1分钟（毫秒值）
	duration = int64(60000)
	// leaseThreshold 需要执行续期逻辑的阀值
	leaseThreshold = int64(1.5 * float64(duration))
	// taskCycle 定时任务周期
	taskCycle = 50000
	// addMulTiExpireScript add multi key expire
	addMulTiExpireScript = `local t = ARGV[1] for i = 1, #KEYS do redis.call('pexpire', KEYS[i], t) end`
)

type Lease struct {
	expire int64
	key    string
}

// getLeaseTime 获取续租时长，返回毫秒值和是否已完成租约
func (lease *Lease) getLeaseTime() (int64, bool) {
	expire := lease.expire
	if expire == permanent {
		return duration, false
	}
	curTime := time.Now().UnixMilli()
	remainingExpire := expire - curTime
	if remainingExpire <= 0 {
		return 0, true
	}
	if remainingExpire >= leaseThreshold {
		return duration, false
	} else {
		return remainingExpire, true
	}
}

type LeaseHolder struct {
	lockSupport LockSupport
	leases      map[string]*Lease
	once        sync.Once
	addChan     chan *Lease
	rmChan      chan string
	closed      bool
}

func newLeaseHolder(lockSupport LockSupport) *LeaseHolder {
	return &LeaseHolder{
		lockSupport: lockSupport,
		leases:      make(map[string]*Lease, 32),
		once:        sync.Once{},
		addChan:     make(chan *Lease, 100),
		rmChan:      make(chan string, 100),
		closed:      false,
	}
}

func (lh *LeaseHolder) addLease(key string, expire int64) {
	lh.initScheduled()
	if lh.closed {
		return
	}
	lh.addChan <- &Lease{
		expire: expire,
		key:    key,
	}
}

func (lh *LeaseHolder) removeLease(key string) {
	if lh.closed {
		return
	}
	lh.rmChan <- key
}

func (lh *LeaseHolder) initScheduled() {
	lh.once.Do(func() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Default().Printf("recover lease scheduled, err:%v", r)
				}
			}()
			ctx := context.Background()
			for !lh.closed {
				select {
				case add := <-lh.addChan:
					lh.leases[add.key] = add
				case rmKey := <-lh.rmChan:
					delete(lh.leases, rmKey)
				case <-time.After(taskCycle * time.Millisecond):
					lh.refreshLease(ctx)
				}
			}
		}()
	})
}

func (lh *LeaseHolder) refreshLease(ctx context.Context) {
	defaultDurationKeys := make([]string, 0)
	waitRemoveKeys := make([]string, 0)
	for key, lease := range lh.leases {
		leaseTime, finish := lease.getLeaseTime()
		if leaseTime == duration {
			defaultDurationKeys = append(defaultDurationKeys, key)
		} else if leaseTime > 0 {
			lh.lockSupport.expire(ctx, key, time.Millisecond*time.Duration(leaseTime))
		}
		if finish {
			waitRemoveKeys = append(waitRemoveKeys, key)
		}
	}
	if len(defaultDurationKeys) > 0 {
		lh.lockSupport.eval(ctx, addMulTiExpireScript, defaultDurationKeys, duration)
	}
	if len(waitRemoveKeys) > 0 {
		for _, key := range waitRemoveKeys {
			delete(lh.leases, key)
		}
	}
}

func (lh *LeaseHolder) close() {
	lh.closed = true
	clear(lh.leases)
	close(lh.addChan)
	close(lh.rmChan)
}
