package fixchain

import (
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/certificate-transparency/go/x509"
)

// FixAndLog contains a Fixer and a Logger, for all your fix-then-log-chain needs!
type FixAndLog struct {
	fixer  *Fixer
	chains chan []*x509.Certificate
	logger *Logger
	wg     sync.WaitGroup

	// Number of whole chains queued - before checking cache & adding chains for intermediate certs.
	queued uint32
	// Cache of chains that QueueAllCertsInChain() has already been called on.
	done *lockedMap
	// Number of whole chains submitted to the FixAndLog using QueueAllCertsInChain() (before adding chains for intermediate certs) that had previously been processed.
	alreadyDone uint32
	// Number of chains queued total, including chains from intermediate certs.
	// Note that in each chain there is len(chain) certs.  So when calling QueueAllCertsInChain(chain), len(chain) chains will actually be added to the queue.
	chainsQueued uint32
	// Number of chains whose leaf cert has already been posted to the log with a valid chain.
	alreadyPosted uint32
	// Number of chains sent on to the Fixer to begin fixing & logging!
	chainsSent uint32
}

// QueueAllCertsInChain adds every cert in the chain and the chain to the queue
// to be fixed and logged.
func (fl *FixAndLog) QueueAllCertsInChain(chain []*x509.Certificate) {
	if chain != nil {
		atomic.AddUint32(&fl.queued, 1)
		atomic.AddUint32(&fl.chainsQueued, uint32(len(chain)))
		dchain := newDedupedChain(chain)
		// Caching check
		h := hashBag(dchain.certs)
		if fl.done.get(h) {
			atomic.AddUint32(&fl.alreadyDone, 1)
			return
		}
		fl.done.set(h, true)

		for _, cert := range dchain.certs {
			if fl.logger.IsPosted(cert) {
				atomic.AddUint32(&fl.alreadyPosted, 1)
				continue
			}
			fl.fixer.QueueChain(cert, dchain.certs, fl.logger.RootCerts())
			atomic.AddUint32(&fl.chainsSent, 1)
		}
	}
}

// QueueChain queues the given chain to be fixed wrt the roots of the logger
// contained in fl, and then logged to the Certificate Transparency log
// represented by the logger.  Note: chain is expected to be in the order of
// cert --> root.
func (fl *FixAndLog) QueueChain(chain []*x509.Certificate) {
	if chain != nil {
		if fl.logger.IsPosted(chain[0]) {
			atomic.AddUint32(&fl.alreadyPosted, 1)
			return
		}
		fl.fixer.QueueChain(chain[0], chain, fl.logger.RootCerts())
		atomic.AddUint32(&fl.chainsSent, 1)
	}
}

// Wait waits for the all of the queued chains to complete being fixed and
// logged.
func (fl *FixAndLog) Wait() {
	fl.fixer.Wait()
	close(fl.chains)
	fl.wg.Wait()
	fl.logger.Wait()
}

// NewFixAndLog creates an object that will asynchronously fix any chains that
// are added to its queue, and then log them to the Certificate Transparency log
// found at the given url.  Any errors encountered along the way are pushed to
// the given errors channel.
func NewFixAndLog(fixerWorkerCount int, loggerWorkerCount int, errors chan<- *FixError, client *http.Client, logClient *http.Client, logURL string, limiter Limiter, logStats bool) *FixAndLog {
	chains := make(chan []*x509.Certificate)
	fl := &FixAndLog{
		fixer:  NewFixer(fixerWorkerCount, chains, errors, client, logStats),
		chains: chains,
		logger: NewLogger(loggerWorkerCount, logURL, errors, logClient, limiter, logStats),
		done:   newLockedMap(),
	}

	fl.wg.Add(1)
	go func() {
		for chain := range chains {
			fl.logger.QueueChain(chain)
		}
		fl.wg.Done()
	}()

	if logStats {
		t := time.NewTicker(time.Second)
		go func() {
			for _ = range t.C {
				log.Printf("fix-then-log: %d whole chains queued, %d whole chains already done, %d total chains queued, %d chains don't need posting (cache hits), %d chains sent to fixer", fl.queued, fl.alreadyDone, fl.chainsQueued, fl.alreadyPosted, fl.chainsSent)
			}
		}()
	}

	return fl
}
