package singleflight

import (
	"log"
	"sync"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	slowJob := func(msg string) ([]byte, error) {
		time.Sleep(time.Millisecond * 500)
		log.Println("hello from slowJob:", msg)
		return []byte(msg), nil
	}

	counter := 0

	job := func() (interface{}, error) {
		counter++
		var ret interface{}
		ret, err := slowJob("114514")
		return ret, err
	}

	g := &Group{}

	wg := sync.WaitGroup{}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(x int) {
			ret, err := g.Do("testDo", job)
			s := string(ret.([]byte))
			if err != nil || s != "114514" || counter != 1 {
				t.Error("unexpected err/ret/counter")
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
}

func TestPanicDo(t *testing.T) {
	slowJob := func(msg string) ([]byte, error) {
		time.Sleep(time.Millisecond * 500)
		log.Panic("panic from slowJob:", msg)
		return []byte(msg), nil
	}

	counter := 0

	job := func() (interface{}, error) {
		counter++
		var ret interface{}
		ret, err := slowJob("114514")
		return ret, err
	}

	g := &Group{}

	wg := sync.WaitGroup{}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(x int) {
			_, err := g.Do("testDo", job)
			if err == nil || counter != 1 {
				t.Error("unexpected err/ret/counter")
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
}
