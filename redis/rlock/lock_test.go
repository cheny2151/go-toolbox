package rlock

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"
)

type itf interface {
	test() int
	test2()
}

type A struct {
}

func (receiver *A) test() int {
	return 0
}

func (receiver *A) test2() {
	fmt.Println(receiver.test())
}

type Combo struct {
	A
}

func (receiver *Combo) test() int {
	return 1
}

func TestCombo(t *testing.T) {
	combo := Combo{}
	combo.test2()
}

func TestBlocking(t *testing.T) {
	cn := make(chan struct{}, 1)
	go func() {
		time.Sleep(time.Second * 2)
		fmt.Println(000)
		cn <- struct{}{}
		close(cn)
		fmt.Println(111)
	}()
	select {
	case <-time.After(time.Second * 3):
		fmt.Println("timeout")
	case <-cn:
		fmt.Println("success")
	}
	time.Sleep(time.Second * 10)
}

func TestLock(t *testing.T) {
	rdb := getRdb()
	lockFactory := NewLockFactory(rdb)
	lock := lockFactory.NewReentrantLock("test")
	group := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() {
			ctx, locked, err := lock.TryLock(context.Background(), 100*time.Second, 10*time.Minute)
			if err != nil {
				t.Errorf("trylock error: %v\n", err)
			}
			time.Sleep(3 * time.Second)
			if locked {
				fmt.Println("locked success")
				err = lock.Unlock(ctx)
				if err != nil {
					t.Errorf("Unlock error: %v\n", err)
				} else {
					fmt.Println("unlocked success")
				}
			}
			group.Done()
		}()
	}
	time.Sleep(3 * time.Second)
	ctx, locked, err := lock.TryLock(context.Background(), 100*time.Second, 10*time.Minute)
	if err != nil {
		t.Errorf("trylock error: %v\n", err)
	}
	if locked {
		fmt.Println("locked success")
		err = lock.Unlock(ctx)
		if err != nil {
			t.Errorf("Unlock error: %v\n", err)
		}
	}
	group.Wait()
	fmt.Println("out")
}

func getRdb() redis.UniversalClient {
	rdb := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    "",
		SentinelAddrs: []string{""},
		DB:            15,
	})
	err := rdb.Ping(context.Background()).Err()
	if err != nil {
		panic(err)
	}
	return rdb
}
