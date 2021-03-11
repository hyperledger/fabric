package commit

import (
	"context"
	"sync"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
)

type LedgerFactory interface {
	Ledger(channelName string) (LedgerNotifier, error)
}

type Notifier struct {
	lock             sync.Mutex
	ledgerFactory    LedgerFactory
	channelNotifiers map[string]*channelNotifier
}

type Notification struct {
	BlockNumber    uint64
	TransactionID  string
	ValidationCode peer.TxValidationCode
}

type LedgerNotifier interface {
	CommitNotificationsChannel(done <-chan struct{}) (<-chan *ledger.CommitNotification, error)
}

func NewNotifier(ledgerFactory LedgerFactory) *Notifier {
	if ledgerFactory == nil {
		panic("nil ledger factory")
	}

	return &Notifier{
		ledgerFactory:    ledgerFactory,
		channelNotifiers: make(map[string]*channelNotifier),
	}
}

func (notifier *Notifier) Notify(done <-chan struct{}, channelName string, transactionID string) (<-chan Notification, error) {
	channelNotifier, err := notifier.channelNotifier(channelName)
	if err != nil {
		return nil, err
	}

	notifyChannel := channelNotifier.RegisterListener(done, transactionID)
	return notifyChannel, nil
}

func (notifier *Notifier) channelNotifier(channelName string) (*channelNotifier, error) {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	result := notifier.channelNotifiers[channelName]
	if result != nil {
		return result, nil
	}

	ledger, err := notifier.ledgerFactory.Ledger(channelName)
	if err != nil {
		return nil, errors.Errorf("channel does not exist: %s", channelName)
	}

	commitChannel, err := ledger.CommitNotificationsChannel(context.Background().Done())
	if err != nil {
		return nil, err
	}

	result = newChannelNotifier(commitChannel)
	notifier.channelNotifiers[channelName] = result

	return result, nil
}

type channelNotifier struct {
	lock            sync.Mutex
	commitChannel   <-chan *ledger.CommitNotification
	listenersByDone map[<-chan struct{}][]*channelListener
	listenersByTx   map[string][]*channelListener
}

type channelListener struct {
	done          <-chan struct{}
	transactionID string
	notifyChannel chan<- Notification
}

func newChannelNotifier(commitChannel <-chan *ledger.CommitNotification) *channelNotifier {
	notifier := &channelNotifier{
		commitChannel:   commitChannel,
		listenersByDone: make(map[<-chan struct{}][]*channelListener),
		listenersByTx:   make(map[string][]*channelListener),
	}
	go notifier.run()
	return notifier
}

func (channelNotifier *channelNotifier) run() {
	for {
		blockCommit, ok := <-channelNotifier.commitChannel
		if !ok {
			break
		}

		for transactionID, status := range blockCommit.TxIDValidationCodes {
			notification := &Notification{
				BlockNumber:    blockCommit.BlockNumber,
				TransactionID:  transactionID,
				ValidationCode: status,
			}
			channelNotifier.notify(notification)
		}
	}
}

func (channelNotifier *channelNotifier) notify(notification *Notification) {
	channelNotifier.lock.Lock()
	defer channelNotifier.lock.Unlock()

	for _, listener := range channelNotifier.listenersByTx[notification.TransactionID] {
		listener.notifyChannel <- *notification
	}

	channelNotifier.removeListenerByTx(notification.TransactionID)
}

func (channelNotifier *channelNotifier) RegisterListener(done <-chan struct{}, transactionID string) <-chan Notification {
	notifyChannel := make(chan Notification, 1) // avoid blocking and only expect one notification per channel
	listener := &channelListener{
		done:          done,
		transactionID: transactionID,
		notifyChannel: notifyChannel,
	}

	channelNotifier.lock.Lock()
	defer channelNotifier.lock.Unlock()

	channelNotifier.listenersByDone[done] = append(channelNotifier.listenersByDone[done], listener)
	channelNotifier.listenersByTx[transactionID] = append(channelNotifier.listenersByTx[transactionID], listener)

	return notifyChannel
}

func (channelNotifier *channelNotifier) removeListenerByDone(done <-chan struct{}) {
	for _, doneListener := range channelNotifier.listenersByDone[done] {
		close(doneListener.notifyChannel)
		channelNotifier.listenersByTx[doneListener.transactionID] = removeListener(channelNotifier.listenersByTx[doneListener.transactionID], doneListener)
	}

	delete(channelNotifier.listenersByDone, done)
}

func (channelNotifier *channelNotifier) removeListenerByTx(transactionID string) {
	for _, txListener := range channelNotifier.listenersByTx[transactionID] {
		close(txListener.notifyChannel)
		channelNotifier.listenersByDone[txListener.done] = removeListener(channelNotifier.listenersByDone[txListener.done], txListener)
	}

	delete(channelNotifier.listenersByTx, transactionID)
}

func removeListener(listeners []*channelListener, listener *channelListener) []*channelListener {
	for i := 0; i < len(listeners); {
		if listeners[i] != listener {
			i++
			continue
		}

		lastIndex := len(listeners) - 1
		listeners[i] = listeners[lastIndex]
		listeners = listeners[:lastIndex]
	}

	return listeners
}
