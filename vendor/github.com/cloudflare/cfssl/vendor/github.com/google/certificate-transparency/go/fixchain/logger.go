package fixchain

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/certificate-transparency/go/x509"
)

// Limiter is an interface to allow different rate limiters to be used with the
// Logger.
type Limiter interface {
	Wait()
}

// Logger contains methods to asynchronously log certificate chains to a
// Certificate Transparency log and properties to store information about each
// attempt that is made to post a certificate chain to said log.
type Logger struct {
	url    string
	client *http.Client
	roots  *x509.CertPool
	toPost chan *toPost
	errors chan<- *FixError

	active uint32

	queued        uint32 // How many chains have been queued to be posted.
	posted        uint32 // How many chains have been posted.
	reposted      uint32 // How many chains for an already-posted cert have been queued.
	chainReposted uint32 // How many chains have been queued again.

	// Note that wg counts the number of active requests, not
	// active servers, because we can't close it to signal the
	// end, because of retries.
	wg      sync.WaitGroup
	limiter Limiter

	postCertCache  *lockedMap
	postChainCache *lockedMap
}

// IsPosted tells the caller whether a chain for the given certificate has
// already been successfully posted to the log by this Logger.
func (l *Logger) IsPosted(cert *x509.Certificate) bool {
	return l.postCertCache.get(hash(cert))
}

// QueueChain adds the given chain to the queue to be posted to the log.
func (l *Logger) QueueChain(chain []*x509.Certificate) {
	if chain == nil {
		return
	}

	atomic.AddUint32(&l.queued, 1)
	// Has a chain for the cert this chain if for already been successfully
	//posted to the log by this Logger?
	h := hash(chain[0]) // Chains are cert -> root
	if l.postCertCache.get(h) {
		atomic.AddUint32(&l.reposted, 1)
		return // Don't post chain for a cert that has already had a chain posted.
	}
	// If we assume all chains for the same cert are equally
	// likely to succeed, then we could mark the cert as posted
	// here. However, bugs might cause a log to refuse one chain
	// and accept another, so try each unique chain.

	// Has this Logger already tried to post this chain?
	h = hashChain(chain)
	if l.postChainCache.get(h) {
		atomic.AddUint32(&l.chainReposted, 1)
		return
	}
	l.postChainCache.set(h, true)

	p := &toPost{chain: chain, retries: 5}
	l.postToLog(p)
}

// Wait for all of the active requests to finish being processed.
func (l *Logger) Wait() {
	l.wg.Wait()
}

// RootCerts returns the root certificates that the log accepts.
func (l *Logger) RootCerts() *x509.CertPool {
	if l.roots == nil {
		// Retry if unable to get roots.
		for i := 0; i < 10; i++ {
			roots, err := l.getRoots()
			if err == nil {
				l.roots = roots
				return l.roots
			}
			log.Println(err)
		}
		log.Fatalf("Can't get roots from %s", l.url)
	}
	return l.roots
}

func (l *Logger) getRoots() (*x509.CertPool, error) {
	rootsJSON, err := l.client.Get(l.url + "/ct/v1/get-roots")
	if err != nil {
		return nil, fmt.Errorf("can't get roots from %s: %s", l.url, err)
	}
	defer rootsJSON.Body.Close()
	j, err := ioutil.ReadAll(rootsJSON.Body)
	if err != nil {
		return nil, fmt.Errorf("can't read body from %s: %s", l.url, err)
	}
	if rootsJSON.StatusCode != 200 {
		return nil, fmt.Errorf("can't deal with status other than 200 from %s: %d\nbody: %s", l.url, rootsJSON.StatusCode, string(j))
	}
	type Certificates struct {
		Certificates [][]byte
	}
	var certs Certificates
	err = json.Unmarshal(j, &certs)
	if err != nil {
		return nil, fmt.Errorf("can't parse json (%s) from %s: %s", err, l.url, j)
	}
	ret := x509.NewCertPool()
	for i := 0; i < len(certs.Certificates); i++ {
		r, err := x509.ParseCertificate(certs.Certificates[i])
		switch err.(type) {
		case nil, x509.NonFatalErrors:
			// ignore
		default:
			return nil, fmt.Errorf("can't parse certificate from %s: %s %#v", l.url, err, certs.Certificates[i])
		}
		ret.AddCert(r)
	}
	return ret, nil
}

type toPost struct {
	chain   []*x509.Certificate
	retries uint8
}

// postToLog(), rather than its asynchronous couterpart asyncPostToLog(), is
// used during the initial queueing of chains to avoid spinning up an excessive
// number of goroutines, and unecessarily using up memory. If asyncPostToLog()
// was called instead, then every time a new chain was queued, a new goroutine
// would be created, each holding their own chain - regardless of whether there
// were postServers available to process them or not.  If a large number of
// chains were queued in a short period of time, this could lead to a large
// number of these additional goroutines being created, resulting in excessive
// memory usage.
func (l *Logger) postToLog(p *toPost) {
	l.wg.Add(1) // Add to the wg as we are adding a new active request to the logger queue.
	l.toPost <- p
}

// asyncPostToLog(), rather than its synchronous couterpart postToLog(), is used
// during retries to avoid deadlock. Without the separate goroutine created in
// asyncPostToLog(), deadlock can occur in the following situation:
//
// Suppose there is only one postServer() goroutine running, and it is blocked
// waiting for a toPost on the toPost chan.  A toPost gets added to the chan,
// which causes the following to happen:
// - the postServer takes the toPost from the chan.
// - the postServer calls l.postChain(toPost), and waits for
//   l.postChain() to return before going back to the toPost
//   chan for another toPost.
// - l.postChain() begins execution.  Suppose the first post
//   attempt of the toPost fails for some network-related
//   reason.
// - l.postChain retries and calls l.postToLog() to queue up the
//   toPost to try to post it again.
// - l.postToLog() tries to put the toPost on the toPost chan,
//   and blocks until a postServer takes it off the chan.
// But the one and only postServer is still waiting for l.postChain (and
// therefore l.postToLog) to return, and will not go to take another toPost off
// the toPost chan until that happens.
// Thus, deadlock.
//
// Similar situations with multiple postServers can easily be imagined.
func (l *Logger) asyncPostToLog(p *toPost) {
	l.wg.Add(1) // Add to the wg as we are adding a new active request to the logger queue.
	go func() {
		l.toPost <- p
	}()
}

func (l *Logger) postChain(p *toPost) {
	h := hash(p.chain[0])
	if l.postCertCache.get(h) {
		atomic.AddUint32(&l.reposted, 1)
		return
	}

	l.limiter.Wait()
	ferr := PostChainToLog(p.chain, l.client, l.url)
	atomic.AddUint32(&l.posted, 1)
	if ferr != nil {
		switch ferr.Type {
		case PostFailed:
			if p.retries == 0 {
				l.errors <- ferr
			} else {
				log.Printf(ferr.Error.Error())
				p.retries--
				l.asyncPostToLog(p)
			}
			return
		case LogPostFailed:
			// If the http error code is 502, we retry.
			// TODO(katjoyce): Are there any other error codes for which the
			// post should be retried?
			if p.retries == 0 || ferr.Code != 502 {
				l.errors <- ferr
			} else {
				p.retries--
				l.asyncPostToLog(p)
			}
			return
		default:
			log.Fatalf("Unexpected FixError type: %s", ferr.TypeString())
		}
	}

	// If the post was successful, cache.
	l.postCertCache.set(h, true)
}

func (l *Logger) postServer() {
	for {
		c := <-l.toPost
		atomic.AddUint32(&l.active, 1)
		l.postChain(c)
		atomic.AddUint32(&l.active, ^uint32(0))
		l.wg.Done()
	}
}

func (l *Logger) logStats() {
	t := time.NewTicker(time.Second)
	go func() {
		for _ = range t.C {
			log.Printf("posters: %d active, %d posted, %d queued, %d certs requeued, %d chains requeued",
				l.active, l.posted, l.queued, l.reposted, l.chainReposted)
		}
	}()
}

// NewLogger creates a new asynchronous logger to log chains to the
// Certificate Transparency log at the given url.  It starts up a pool of
// workerCount workers.  Errors are pushed to the errors channel.  client is
// used to post the chains to the log.
func NewLogger(workerCount int, url string, errors chan<- *FixError, client *http.Client, limiter Limiter, logStats bool) *Logger {
	l := &Logger{
		url:            url,
		client:         client,
		errors:         errors,
		toPost:         make(chan *toPost),
		postCertCache:  newLockedMap(),
		postChainCache: newLockedMap(),
		limiter:        limiter,
	}
	l.RootCerts()

	// Start post server pool.
	for i := 0; i < workerCount; i++ {
		go l.postServer()
	}

	if logStats {
		l.logStats()
	}
	return l
}
