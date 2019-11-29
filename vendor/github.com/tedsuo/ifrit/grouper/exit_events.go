package grouper

import (
	"fmt"
	"sync"
)

/*
An ExitEvent occurs every time an invoked member exits.
*/
type ExitEvent struct {
	Member Member
	Err    error
}

type exitEventChannel chan ExitEvent

func newExitEventChannel(bufferSize int) exitEventChannel {
	return make(exitEventChannel, bufferSize)
}

type exitEventBroadcaster struct {
	channels   []exitEventChannel
	buffer     slidingBuffer
	bufferSize int
	lock       *sync.Mutex
}

func newExitEventBroadcaster(bufferSize int) *exitEventBroadcaster {
	return &exitEventBroadcaster{
		channels:   make([]exitEventChannel, 0),
		buffer:     newSlidingBuffer(bufferSize),
		bufferSize: bufferSize,
		lock:       new(sync.Mutex),
	}
}

func (b *exitEventBroadcaster) Attach() exitEventChannel {
	b.lock.Lock()
	defer b.lock.Unlock()

	channel := newExitEventChannel(b.bufferSize)
	b.buffer.Range(func(event interface{}) {
		channel <- event.(ExitEvent)
	})
	if b.channels != nil {
		b.channels = append(b.channels, channel)
	} else {
		close(channel)
	}
	return channel
}

func (b *exitEventBroadcaster) Broadcast(exit ExitEvent) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.buffer.Append(exit)
	for _, exitChan := range b.channels {
		exitChan <- exit
	}
}

func (b *exitEventBroadcaster) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, channel := range b.channels {
		close(channel)
	}
	b.channels = nil
}

type ErrorTrace []ExitEvent

func (trace ErrorTrace) Error() string {
	msg := "Exit trace for group:\n"

	for _, exit := range trace {
		if exit.Err == nil {
			msg += fmt.Sprintf("%s exited with nil\n", exit.Member.Name)
		} else {
			msg += fmt.Sprintf("%s exited with error: %s\n", exit.Member.Name, exit.Err.Error())
		}
	}

	return msg
}

func (trace ErrorTrace) ErrorOrNil() error {
	for _, exit := range trace {
		if exit.Err != nil {
			return trace
		}
	}

	return nil
}
