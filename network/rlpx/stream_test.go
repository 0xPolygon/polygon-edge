package rlpx

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

type timestamp struct {
	t    time.Duration
	code uint64
}

type timestamps []*timestamp

func (t *timestamps) Copy() timestamps {
	tt := []*timestamp{}
	for _, i := range *t {
		tt = append(tt, &timestamp{i.t, i.code})
	}
	return tt
}

func (t timestamps) Len() int           { return len(t) }
func (t timestamps) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t timestamps) Less(i, j int) bool { return t[i].t.Nanoseconds() < t[j].t.Nanoseconds() }

func TestMultipleStreamHandlers(t *testing.T) {
	s := NewStream(0, 0, nil)

	tt := timestamps{
		{1 * time.Second, 0x1},
		{500 * time.Millisecond, 0x2},
		{200 * time.Millisecond, 0x3},
		{700 * time.Millisecond, 0x4},
	}

	expected := tt.Copy()
	sort.Sort(expected)

	now := time.Now()

	ack := make(chan AckMessage, len(tt))
	for _, i := range tt {
		s.SetHandler(i.code, ack, i.t)
	}

	span := time.Duration(100 * time.Millisecond).Nanoseconds()

	for _, i := range expected {
		a := <-ack
		if a.Code != i.code {
			t.Fatal("bad")
		}

		elapsed := time.Now().Sub(now)

		d := elapsed.Nanoseconds() - i.t.Nanoseconds()
		if d < 0 {
			d = d * -1
		}

		if d > span {
			t.Fatal("bad")
		}
	}
}

func TestStreamHandlerUpdate(t *testing.T) {
	s := NewStream(0, 0, nil)

	ack0 := make(chan AckMessage, 1)
	s.SetHandler(1, ack0, 500*time.Millisecond)

	ack1 := make(chan AckMessage, 1)
	s.SetHandler(1, ack1, 700*time.Millisecond)

	select {
	case <-ack1:
		// good. seconds handler updates the first one
	case <-ack0:
		t.Fatal("it should have been updated")
	case <-time.After(1 * time.Second):
		t.Fatal("bad")
	}
}

func TestStreamMessages(t *testing.T) {
	c0, c1 := pipe(t)
	defer c0.Close()
	defer c1.Close()

	s0 := c0.OpenStream(5, 10)
	s1 := c1.OpenStream(5, 10)

	go func() {
		if err := s0.WriteMsg(0x1); err != nil {
			panic(err)
		}
	}()

	x, err := s1.ReadMsg()
	if err != nil {
		panic(err)
	}

	fmt.Println(x)
}

func TestStreamMessagesWrongStream(t *testing.T) {
	c0, c1 := pipe(t)
	defer c0.Close()
	defer c1.Close()

	s0 := c0.OpenStream(5, 10)
	s1 := c1.OpenStream(5, 10)

	go func() {
		if err := s0.WriteMsg(0x1); err != nil {
			panic(err)
		}
	}()

	x, err := s1.ReadMsg()
	if err != nil {
		panic(err)
	}

	fmt.Println("-- code --")
	fmt.Println(x.Code)
}

func TestSessionMultipleStreams(t *testing.T) {

}
