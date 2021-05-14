/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package mirbft is a consensus library, implementing the Mir BFT consensus protocol.
//
// This library can be used by applications which desire distributed, byzantine fault
// tolerant consensus on message order.  Unlike many traditional consensus algorithms,
// Mir BFT is a multi-leader protocol, allowing throughput to scale with the number of
// nodes (even over WANs), where the performance of traditional consenus algorithms
// begin to degrade.
package mirbft

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger-labs/mirbft/pkg/pb/msgs"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
	"github.com/hyperledger-labs/mirbft/pkg/processor"
	"github.com/hyperledger-labs/mirbft/pkg/statemachine"
	"github.com/hyperledger-labs/mirbft/pkg/status"
	"github.com/pkg/errors"
)

var ErrStopped = fmt.Errorf("stopped at caller request")

type replicas struct {
	mutex    sync.Mutex
	eventC   chan *statemachine.EventList
	replicas processor.Replicas
}

func (r *replicas) replica(id uint64) *processor.Replica {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.replicas.Replica(id)
}

type Client struct {
	client          *processor.Client
	resultC         chan<- *statemachine.EventList
	workErrNotifier *workErrNotifier
}

func (c *Client) NextReqNo() (uint64, error) {
	return c.client.NextReqNo()
}

func (c *Client) Propose(ctx context.Context, reqNo uint64, data []byte) error {

	result, err := c.client.Propose(reqNo, data)
	if err != nil {
		return err
	}

	select {
	case c.resultC <- result:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.workErrNotifier.ExitC():
		return c.workErrNotifier.Err()
	}
}

// Node is the local instance of the MirBFT state machine through which the calling application
// proposes new messages, receives delegated actions, and returns action results.
// The methods exposed on Node are all thread safe, though typically, a single loop handles
// reading Actions, writing results, and writing ticks, while other go routines Propose and Step.
type Node struct {
	ID              uint64
	Config          *Config
	processorConfig *ProcessorConfig

	replicas *replicas

	stateMachine    *statemachine.StateMachine
	workItems       *processor.WorkItems
	workErrNotifier *workErrNotifier

	statusC          chan chan *status.StateMachine
	walActionsC      chan *statemachine.ActionList
	walResultsC      chan *statemachine.ActionList
	clientActionsC   chan *statemachine.ActionList
	clientResultsC   chan *statemachine.EventList
	hashActionsC     chan *statemachine.ActionList
	hashResultsC     chan *statemachine.EventList
	netActionsC      chan *statemachine.ActionList
	netResultsC      chan *statemachine.EventList
	appActionsC      chan *statemachine.ActionList
	appResultsC      chan *statemachine.EventList
	reqStoreEventsC  chan *statemachine.EventList
	reqStoreResultsC chan *statemachine.EventList
	resultEventsC    chan *statemachine.EventList
	resultResultsC   chan *statemachine.ActionList
	clients          *processor.Clients
}

func StandardInitialNetworkState(nodeCount int, clientCount int) *msgs.NetworkState {
	nodes := []uint64{}
	for i := 0; i < nodeCount; i++ {
		nodes = append(nodes, uint64(i))
	}

	numberOfBuckets := int32(nodeCount)
	checkpointInterval := numberOfBuckets * 5
	maxEpochLength := checkpointInterval * 10

	clients := make([]*msgs.NetworkState_Client, clientCount)
	for i := range clients {
		clients[i] = &msgs.NetworkState_Client{
			Id:           uint64(i),
			Width:        100,
			LowWatermark: 0,
		}
	}

	return &msgs.NetworkState{
		Config: &msgs.NetworkState_Config{
			Nodes:              nodes,
			F:                  int32((nodeCount - 1) / 3),
			NumberOfBuckets:    numberOfBuckets,
			CheckpointInterval: checkpointInterval,
			MaxEpochLength:     uint64(maxEpochLength),
		},
		Clients: clients,
	}
}

// NewNode creates a new node.  The processor must be started either by invoking
// node.Processor.StartNewNode with the initial state or by invoking node.Processor.RestartNode.
func NewNode(
	id uint64,
	config *Config,
	processorConfig *ProcessorConfig,
) (*Node, error) {
	return &Node{
		ID:              id,
		Config:          config,
		processorConfig: processorConfig,

		replicas: &replicas{
			eventC: make(chan *statemachine.EventList),
		},
		stateMachine: &statemachine.StateMachine{
			Logger: logAdapter{Logger: config.Logger},
		},
		clients: &processor.Clients{
			RequestStore: processorConfig.RequestStore,
			Hasher:       processorConfig.Hasher,
		},
		workItems:       processor.NewWorkItems(),
		workErrNotifier: newWorkErrNotifier(),

		statusC:          make(chan chan *status.StateMachine),
		walActionsC:      make(chan *statemachine.ActionList),
		walResultsC:      make(chan *statemachine.ActionList),
		clientActionsC:   make(chan *statemachine.ActionList),
		clientResultsC:   make(chan *statemachine.EventList),
		hashActionsC:     make(chan *statemachine.ActionList),
		hashResultsC:     make(chan *statemachine.EventList),
		netActionsC:      make(chan *statemachine.ActionList),
		netResultsC:      make(chan *statemachine.EventList),
		appActionsC:      make(chan *statemachine.ActionList),
		appResultsC:      make(chan *statemachine.EventList),
		reqStoreEventsC:  make(chan *statemachine.EventList),
		reqStoreResultsC: make(chan *statemachine.EventList),
		resultEventsC:    make(chan *statemachine.EventList),
		resultResultsC:   make(chan *statemachine.ActionList),
	}, nil
}

// Status returns a static snapshot in time of the internal state of the state machine.
// This method necessarily exposes some of the internal architecture of the system, and
// especially while the library is in development, the data structures may change substantially.
// This method returns a nil status and an error if the context ends.  If the serializer go routine
// exits for any other reason, then a best effort is made to return the last (and final) status.
// This final status may be relied upon if it is non-nil.  If the serializer exited at the user's
// request (because the done channel was closed), then ErrStopped is returned.
func (n *Node) Status(ctx context.Context) (*status.StateMachine, error) {
	statusC := make(chan *status.StateMachine, 1)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.workErrNotifier.ExitStatusC():
		return n.workErrNotifier.ExitStatus()
	case n.statusC <- statusC:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.workErrNotifier.ExitStatusC():
		return n.workErrNotifier.ExitStatus()
	case s := <-statusC:
		return s, nil
	}
}

func (n *Node) Step(ctx context.Context, source uint64, msg *msgs.Msg) error {
	r := n.replicas.replica(source)

	e, err := r.Step(msg)
	if err != nil {
		return err
	}

	select {
	case n.replicas.eventC <- e:
		return nil
	case <-n.workErrNotifier.ExitStatusC():
		return n.workErrNotifier.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Node) Client(id uint64) *Client {
	return &Client{
		client:          n.clients.Client(id),
		resultC:         n.clientResultsC,
		workErrNotifier: n.workErrNotifier,
	}
}

func (n *Node) doWALWork(exitC <-chan struct{}) error {
	var actions *statemachine.ActionList
	select {
	case actions = <-n.walActionsC:
	case <-exitC:
		return ErrStopped
	}

	walResults, err := processor.ProcessWALActions(n.processorConfig.WAL, actions)
	if err != nil {
		return errors.WithMessage(err, "could not perform WAL actions")
	}

	if walResults.Len() == 0 {
		return nil
	}

	select {
	case n.walResultsC <- walResults:
	case <-exitC:
		return ErrStopped
	}

	return nil
}

func (n *Node) doClientWork(exitC <-chan struct{}) error {
	var actions *statemachine.ActionList
	select {
	case actions = <-n.clientActionsC:
	case <-exitC:
		return ErrStopped
	}

	clientResults, err := n.clients.ProcessClientActions(actions)
	if err != nil {
		return errors.WithMessage(err, "could not perform client actions")
	}

	if clientResults.Len() == 0 {
		return nil
	}

	select {
	case n.clientResultsC <- clientResults:
	case <-exitC:
		return ErrStopped
	}
	return nil
}

func (n *Node) doHashWork(exitC <-chan struct{}) error {
	var actions *statemachine.ActionList
	select {
	case actions = <-n.hashActionsC:
	case <-exitC:
		return ErrStopped
	}

	hashResults, err := processor.ProcessHashActions(n.processorConfig.Hasher, actions)
	if err != nil {
		return errors.WithMessage(err, "could not perform hash actions")
	}

	select {
	case n.hashResultsC <- hashResults:
	case <-exitC:
		return ErrStopped
	}

	return nil
}

func (n *Node) doNetWork(exitC <-chan struct{}) error {
	var actions *statemachine.ActionList
	select {
	case actions = <-n.netActionsC:
	case <-exitC:
		return ErrStopped
	}

	netResults, err := processor.ProcessNetActions(n.ID, n.processorConfig.Link, actions)
	if err != nil {
		return errors.WithMessage(err, "could not perform net actions")
	}

	select {
	case n.netResultsC <- netResults:
	case <-exitC:
		return ErrStopped
	}

	return nil
}

func (n *Node) doAppWork(exitC <-chan struct{}) error {
	var actions *statemachine.ActionList
	select {
	case actions = <-n.appActionsC:
	case <-exitC:
		return ErrStopped
	}

	appResults, err := processor.ProcessAppActions(n.processorConfig.App, actions)
	if err != nil {
		return errors.WithMessage(err, "could not perform app actions")
	}

	select {
	case n.appResultsC <- appResults:
	case <-exitC:
		return ErrStopped
	}

	return nil
}

func (n *Node) doReqStoreWork(exitC <-chan struct{}) error {
	var events *statemachine.EventList
	select {
	case events = <-n.reqStoreEventsC:
	case <-exitC:
		return ErrStopped
	}

	reqStoreResults, err := processor.ProcessReqStoreEvents(n.processorConfig.RequestStore, events)
	if err != nil {
		return errors.WithMessage(err, "could not perform reqstore actions")
	}

	select {
	case n.reqStoreResultsC <- reqStoreResults:
	case <-exitC:
		return ErrStopped
	}

	return nil
}

func (n *Node) doStateMachineWork(exitC <-chan struct{}) (err error) {
	defer func() {
		if err != nil {
			s, err := n.stateMachine.Status()
			n.workErrNotifier.SetExitStatus(s, err)
		}
	}()

	var events *statemachine.EventList
	select {
	case events = <-n.resultEventsC:
	case <-exitC:
		return ErrStopped
	}

	actions, err := processor.ProcessStateMachineEvents(n.stateMachine, n.processorConfig.Interceptor, events)
	if err != nil {
		return err
	}

	if actions.Len() == 0 {
		return nil
	}

	select {
	case n.resultResultsC <- actions:
		n.processorConfig.Interceptor.Intercept(statemachine.EventActionsReceived())
	case <-exitC:
		return ErrStopped
	}

	return nil
}

type workFunc func(exitC <-chan struct{}) error

func (n *Node) doUntilErr(work workFunc) {
	for {
		err := work(n.workErrNotifier.ExitC())
		if err != nil {
			n.workErrNotifier.Fail(err)
			return
		}
	}
}

type ProcessorConfig struct {
	Link         processor.Link
	Hasher       processor.Hasher
	App          processor.App
	WAL          processor.WAL
	RequestStore processor.RequestStore
	Interceptor  processor.EventInterceptor
}

func (n *Node) runtimeParms() *state.EventInitialParameters {
	return &state.EventInitialParameters{
		Id:                   n.ID,
		BatchSize:            n.Config.BatchSize,
		HeartbeatTicks:       n.Config.HeartbeatTicks,
		SuspectTicks:         n.Config.SuspectTicks,
		NewEpochTimeoutTicks: n.Config.NewEpochTimeoutTicks,
		BufferSize:           n.Config.BufferSize,
	}
}

func (n *Node) ProcessAsNewNode(
	exitC <-chan struct{},
	tickC <-chan time.Time,
	initialNetworkState *msgs.NetworkState,
	initialCheckpointValue []byte,
) error {
	events, err := processor.IntializeWALForNewNode(n.processorConfig.WAL, n.runtimeParms(), initialNetworkState, initialCheckpointValue)
	if err != nil {
		n.workErrNotifier.SetExitStatus(nil, errors.Errorf("state machine was not started"))
		return err
	}

	n.workItems.ResultEvents().PushBackList(events)
	return n.process(exitC, tickC)
}

func (n *Node) RestartProcessing(
	exitC <-chan struct{},
	tickC <-chan time.Time,
) error {
	events, err := processor.RecoverWALForExistingNode(n.processorConfig.WAL, n.runtimeParms())
	if err != nil {
		n.workErrNotifier.SetExitStatus(nil, errors.Errorf("state machine was not started"))
		return err
	}

	n.workItems.ResultEvents().PushBackList(events)
	return n.process(exitC, tickC)
}
func (n *Node) process(exitC <-chan struct{}, tickC <-chan time.Time) error {
	var wg sync.WaitGroup
	for _, work := range []workFunc{
		n.doWALWork,
		n.doClientWork,
		n.doHashWork, // TODO, spawn more of these
		n.doNetWork,
		n.doAppWork,
		n.doReqStoreWork,
		n.doStateMachineWork,
	} {
		wg.Add(1)
		go func(work workFunc) {
			wg.Done()
			n.doUntilErr(work)
		}(work)
	}
	defer wg.Wait()

	var walActionsC, clientActionsC, hashActionsC, netActionsC, appActionsC chan<- *statemachine.ActionList
	var reqStoreEventsC, resultEventsC chan<- *statemachine.EventList

	for {
		select {
		case resultEventsC <- n.workItems.ResultEvents():
			n.workItems.ClearResultEvents()
			resultEventsC = nil
		case walActionsC <- n.workItems.WALActions():
			n.workItems.ClearWALActions()
			walActionsC = nil
		case walResultsC := <-n.walResultsC:
			n.workItems.AddWALResults(walResultsC)
		case clientActionsC <- n.workItems.ClientActions():
			n.workItems.ClearClientActions()
			clientActionsC = nil
		case hashActionsC <- n.workItems.HashActions():
			n.workItems.ClearHashActions()
			hashActionsC = nil
		case netActionsC <- n.workItems.NetActions():
			n.workItems.ClearNetActions()
			netActionsC = nil
		case appActionsC <- n.workItems.AppActions():
			n.workItems.ClearAppActions()
			appActionsC = nil
		case reqStoreEventsC <- n.workItems.ReqStoreEvents():
			n.workItems.ClearReqStoreEvents()
			reqStoreEventsC = nil
		case clientResults := <-n.clientResultsC:
			n.workItems.AddClientResults(clientResults)
		case hashResults := <-n.hashResultsC:
			n.workItems.AddHashResults(hashResults)
		case netResults := <-n.netResultsC:
			n.workItems.AddNetResults(netResults)
		case appResults := <-n.appResultsC:
			n.workItems.AddAppResults(appResults)
		case reqStoreResults := <-n.reqStoreResultsC:
			n.workItems.AddReqStoreResults(reqStoreResults)
		case actions := <-n.resultResultsC:
			n.workItems.AddStateMachineResults(actions)
		case stepEvents := <-n.replicas.eventC:
			// TODO, once request forwarding works, we'll
			// need to split this into the req store component
			// and 'other' that goes into the result events.
			n.workItems.ResultEvents().PushBackList(stepEvents)
		case <-n.workErrNotifier.ExitC():
			return n.workErrNotifier.Err()
		case <-tickC:
			n.workItems.ResultEvents().TickElapsed()
		case <-exitC:
			n.workErrNotifier.Fail(ErrStopped)
		}

		if resultEventsC == nil && n.workItems.ResultEvents().Len() > 0 {
			resultEventsC = n.resultEventsC
		}

		if walActionsC == nil && n.workItems.WALActions().Len() > 0 {
			walActionsC = n.walActionsC
		}

		if clientActionsC == nil && n.workItems.ClientActions().Len() > 0 {
			clientActionsC = n.clientActionsC
		}

		if hashActionsC == nil && n.workItems.HashActions().Len() > 0 {
			hashActionsC = n.hashActionsC
		}

		if netActionsC == nil && n.workItems.NetActions().Len() > 0 {
			netActionsC = n.netActionsC
		}

		if appActionsC == nil && n.workItems.AppActions().Len() > 0 {
			appActionsC = n.appActionsC
		}

		if reqStoreEventsC == nil && n.workItems.ReqStoreEvents().Len() > 0 {
			reqStoreEventsC = n.reqStoreEventsC
		}
	}
}

// workErrNotifier is used to synchronize the exit of the assorted worker
// go routines.  The first worker to encounter an error should call Fail(err),
// then the other workers will (eventually) read ExitC() to determine that they
// should exit.  The worker thread responsible for the state machine _must_
// call SetExitStatus(status, statusErr) before returning.
type workErrNotifier struct {
	mutex         sync.Mutex
	err           error
	exitC         chan struct{}
	exitStatus    *status.StateMachine
	exitStatusErr error
	exitStatusC   chan struct{}
}

func newWorkErrNotifier() *workErrNotifier {
	return &workErrNotifier{
		exitC:       make(chan struct{}),
		exitStatusC: make(chan struct{}),
	}
}

func (wen *workErrNotifier) Err() error {
	wen.mutex.Lock()
	defer wen.mutex.Unlock()
	return wen.err
}

func (wen *workErrNotifier) Fail(err error) {
	wen.mutex.Lock()
	defer wen.mutex.Unlock()
	if wen.err != nil {
		return
	}
	wen.err = err
	close(wen.exitC)
}

func (wen *workErrNotifier) SetExitStatus(s *status.StateMachine, err error) {
	wen.mutex.Lock()
	defer wen.mutex.Unlock()
	wen.exitStatus = s
	wen.exitStatusErr = err
	close(wen.exitStatusC)
}

func (wen *workErrNotifier) ExitStatus() (*status.StateMachine, error) {
	wen.mutex.Lock()
	defer wen.mutex.Unlock()
	return wen.exitStatus, wen.exitStatusErr
}

func (wen *workErrNotifier) ExitC() <-chan struct{} {
	return wen.exitC
}

func (wen *workErrNotifier) ExitStatusC() <-chan struct{} {
	return wen.exitStatusC
}
