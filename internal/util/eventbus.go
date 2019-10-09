package util

import (
	"k8s.io/klog"
	"sync"
)

// EventBus is a simple go event bus.
//
// EventBus makes no guarantees of delivery ordering.
type EventBus struct {
	subscribers map[string]*chanList
	lock        sync.RWMutex
}

type Data = interface{}

type Event struct {
	Topic   string
	Message Data
}

type chanList struct {
	list map[uint]chan<- Event
	next uint
}

type Unsubscriber = func()

func NewEventBus() *EventBus {
	bus := &EventBus{
		subscribers: map[string]*chanList{},
	}

	return bus
}

func (b *EventBus) Subscribe(topic string, ch chan<- Event) Unsubscriber {
	b.lock.Lock()
	defer b.lock.Unlock()

	if _, ok := b.subscribers[topic]; !ok {
		b.subscribers[topic] = &chanList{
			list: map[uint]chan<- Event{},
			next: 0,
		}
	}

	curNum := b.subscribers[topic].next
	b.subscribers[topic].list[curNum] = ch
	b.subscribers[topic].next++

	return func() {
		b.lock.Lock()
		defer b.lock.Unlock()

		delete(b.subscribers[topic].list, curNum)
	}
}

func (b *EventBus) Publish(topic string, message Data) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if chanlist, ok := b.subscribers[topic]; ok {
		// Making a copy to ensure that no modifications to the array are made.
		var channels []chan<- Event
		for _, ch := range chanlist.list {
			channels = append(channels, ch)
		}
		go func(chans []chan<- Event, event Event) {
			for _, chn := range chans {
				// These can hang forever if there is nothing waiting on the other side.
				go func(ch chan<- Event) {
					defer func() {
						if r := recover(); r != nil {
							klog.Warningln("Recovered while writing event to %s: %v", event.Topic, r)
						}
					}()
					ch <- event
				}(chn)
			}
		}(channels, Event{topic, message})
	}
}

var mainBus = NewEventBus()

func Subscribe(topic string, ch chan Event) Unsubscriber {
	return mainBus.Subscribe(topic, ch)
}

func Publish(topic string, message Data) {
	mainBus.Publish(topic, message)
}
