package grouper

import "container/list"

type slidingBuffer struct {
	buffer   *list.List
	capacity int
}

func newSlidingBuffer(capacity int) slidingBuffer {
	return slidingBuffer{list.New(), capacity}
}

func (b slidingBuffer) Append(item interface{}) {
	if b.capacity == 0 {
		return
	}

	b.buffer.PushBack(item)
	if b.buffer.Len() > b.capacity {
		b.buffer.Remove(b.buffer.Front())
	}
}

func (b slidingBuffer) Range(callback func(item interface{})) {
	elem := b.buffer.Front()
	for elem != nil {
		callback(elem.Value)
		elem = elem.Next()
	}
}

func (b slidingBuffer) Length() int {
	return b.buffer.Len()
}
