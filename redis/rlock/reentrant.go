package rlock

import (
	"context"
	"time"
)

const (
	reentrantPrefixKey = "REENTRANT_LOCK"
	reentrantLockLua   = `if (redis.call('exists', KEYS[1]) == 0) then
		redis.call('hset', KEYS[1], ARGV[2], 1);
		if (tonumber(ARGV[1]) > 0) then
		redis.call('pexpire', KEYS[1], ARGV[1]);
		end;
		return nil;
		end;
		if (redis.call('hexists', KEYS[1], ARGV[2]) == 1) then
		redis.call('hincrby', KEYS[1], ARGV[2], 1);
		if (tonumber(ARGV[1]) > 0) then
		redis.call('pexpire', KEYS[1], ARGV[1]);
		end;
		return nil;
		end;
		return redis.call('pttl', KEYS[1]);`
	reentrantUnlockLua = `if (redis.call('exists', KEYS[1]) == 0) then
		redis.call('publish', KEYS[2], ARGV[1]);
		return 1;
		end;
		if (redis.call('hexists', KEYS[1], ARGV[3]) == 0) then
		return nil;
		end;
		local counter = redis.call('hincrby', KEYS[1], ARGV[3], -1);
		if (counter > 0) then
		if (tonumber(ARGV[2]) > 0) then
		redis.call('pexpire', KEYS[1], ARGV[2]);
		end;
		return 0;
		else
		redis.call('del', KEYS[1]);
		redis.call('publish', KEYS[2], ARGV[1]);
		return 1;
		end;
		return nil;`
)

type ReentrantLock struct {
	LockSupport
	key  string
	path string
}

func (rl *ReentrantLock) tryLock(ctx context.Context, waitTime, leaseTime time.Duration) (context.Context, bool, error) {
	return rl.tryLock0(ctx, waitTime, leaseTime, rl.lockArgs)
}

func (rl *ReentrantLock) lockArgs(waitTime, leaseTime time.Duration, lockId string) (path, lua string, keys []string, argv []any) {
	path = rl.path
	lua = reentrantLockLua
	keys = []string{path}
	argv = []any{leaseTime.Milliseconds(), lockId}
	return
}

func (rl *ReentrantLock) unlock(ctx context.Context) error {
	return rl.unlock0(ctx, rl.unlockArgs)
}

func (rl *ReentrantLock) unlockArgs(lockId string) (lua string, keys []string, argv []any) {
	lua = reentrantUnlockLua
	keys = []string{rl.path, lockChannel + rl.path}
	argv = []any{rl.path, 0, lockId}
	return
}
