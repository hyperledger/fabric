package grouper

import (
	"sync"

	"github.com/tedsuo/ifrit"
)

/*
DynamicClient provides a client with group controls and event notifications.
A client can use the insert channel to add members to the group. When the group
becomes full, the insert channel blocks until a running process exits the group.
Once there are no more members to be added, the client can close the dynamic
group, preventing new members from being added.
*/
type DynamicClient interface {

	/*
	   EntranceListener provides a new buffered channel of entrance events, which are
	   emited every time an inserted process is ready. To help prevent race conditions,
	   every new channel is populated with previously emited events, up to it's buffer
	   size.
	*/
	EntranceListener() <-chan EntranceEvent

	/*
	   ExitListener provides a new buffered channel of exit events, which are emited
	   every time an inserted process exits. To help prevent race conditions, every
	   new channel is populated with previously emited events, up to it's buffer size.
	*/
	ExitListener() <-chan ExitEvent

	/*
	   CloseNotifier provides a new unbuffered channel, which will emit a single event
	   once the group has been closed.
	*/
	CloseNotifier() <-chan struct{}
	/*
	   Inserter provides an unbuffered channel for adding members to a group. When the
	   group becomes full, the insert channel blocks until a running process exits.
	   Once the group is closed, insert channels block forever.
	*/
	Inserter() chan<- Member

	/*
	   Close causes a dynamic group to become a static group. This means that no new
	   members may be inserted, and the group will exit once all members have
	   completed.
	*/
	Close()

	Get(name string) (ifrit.Process, bool)
}

type memberRequest struct {
	Name     string
	Response chan ifrit.Process
}

/*
dynamicClient implements DynamicClient.
*/
type dynamicClient struct {
	insertChannel       chan Member
	getMemberChannel    chan memberRequest
	completeNotifier    chan struct{}
	closeNotifier       chan struct{}
	closeOnce           *sync.Once
	entranceBroadcaster *entranceEventBroadcaster
	exitBroadcaster     *exitEventBroadcaster
}

func newClient(bufferSize int) dynamicClient {
	return dynamicClient{
		insertChannel:       make(chan Member),
		getMemberChannel:    make(chan memberRequest),
		completeNotifier:    make(chan struct{}),
		closeNotifier:       make(chan struct{}),
		closeOnce:           new(sync.Once),
		entranceBroadcaster: newEntranceEventBroadcaster(bufferSize),
		exitBroadcaster:     newExitEventBroadcaster(bufferSize),
	}
}

func (c dynamicClient) Get(name string) (ifrit.Process, bool) {
	req := memberRequest{
		Name:     name,
		Response: make(chan ifrit.Process),
	}
	select {
	case c.getMemberChannel <- req:
		p, ok := <-req.Response
		if !ok {
			return nil, false
		}
		return p, true
	case <-c.completeNotifier:
		return nil, false
	}
}

func (c dynamicClient) memberRequests() chan memberRequest {
	return c.getMemberChannel
}

func (c dynamicClient) Inserter() chan<- Member {
	return c.insertChannel
}

func (c dynamicClient) insertEventListener() <-chan Member {
	return c.insertChannel
}

func (c dynamicClient) EntranceListener() <-chan EntranceEvent {
	return c.entranceBroadcaster.Attach()
}

func (c dynamicClient) broadcastEntrance(event EntranceEvent) {
	c.entranceBroadcaster.Broadcast(event)
}

func (c dynamicClient) closeEntranceBroadcaster() {
	c.entranceBroadcaster.Close()
}

func (c dynamicClient) ExitListener() <-chan ExitEvent {
	return c.exitBroadcaster.Attach()
}

func (c dynamicClient) broadcastExit(event ExitEvent) {
	c.exitBroadcaster.Broadcast(event)
}

func (c dynamicClient) closeExitBroadcaster() {
	c.exitBroadcaster.Close()
}

func (c dynamicClient) closeBroadcasters() error {
	c.entranceBroadcaster.Close()
	c.exitBroadcaster.Close()
	close(c.completeNotifier)
	return nil
}

func (c dynamicClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closeNotifier)
	})
}

func (c dynamicClient) CloseNotifier() <-chan struct{} {
	return c.closeNotifier
}
