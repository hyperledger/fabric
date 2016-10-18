package fixchain

import (
	"bytes"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/certificate-transparency/go/x509"
)

// Fixer contains methods to asynchronously fix certificate chains and
// properties to store information about each attempt that is made to fix a
// certificate chain.
type Fixer struct {
	toFix  chan *toFix
	chains chan<- []*x509.Certificate // Chains successfully fixed by the fixer
	errors chan<- *FixError

	active uint32

	reconstructed       uint32
	notReconstructed    uint32
	fixed               uint32
	notFixed            uint32
	validChainsProduced uint32
	validChainsOut      uint32

	wg    sync.WaitGroup
	cache *urlCache
}

// QueueChain adds the given cert and chain to the queue to be fixed by the
// fixer, with respect to the given roots.  Note: chain is expected to be in the
// order of cert --> root.
func (f *Fixer) QueueChain(cert *x509.Certificate, chain []*x509.Certificate, roots *x509.CertPool) {
	f.toFix <- &toFix{
		cert:  cert,
		chain: newDedupedChain(chain),
		roots: roots,
		cache: f.cache,
	}
}

// Wait for all the fixer workers to finish.
func (f *Fixer) Wait() {
	close(f.toFix)
	f.wg.Wait()
}

func (f *Fixer) updateCounters(chains [][]*x509.Certificate, ferrs []*FixError) {
	atomic.AddUint32(&f.validChainsProduced, uint32(len(chains)))

	var verifyFailed bool
	var fixFailed bool
	for _, ferr := range ferrs {
		switch ferr.Type {
		case VerifyFailed:
			verifyFailed = true
		case FixFailed:
			fixFailed = true
		}
	}
	// No errors --> reconstructed
	// VerifyFailed --> notReconstructed
	// VerifyFailed but no FixFailed --> fixed
	// VerifyFailed and FixFailed --> notFixed
	if verifyFailed {
		atomic.AddUint32(&f.notReconstructed, 1)
		// FixFailed error will only be present if a VerifyFailed error is, as
		// fixChain() is only called if constructChain() fails.
		if fixFailed {
			atomic.AddUint32(&f.notFixed, 1)
			return
		}
		atomic.AddUint32(&f.fixed, 1)
		return
	}
	atomic.AddUint32(&f.reconstructed, 1)
}

type chainSlice struct {
	chains [][]*x509.Certificate
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sort.Sort(data Interface) for chainSlice - uses data.Len, data.Less & data.Swap.
// Sort will sort the chains contained within the chainSlice.  The chains will
// be sorted in order of the first certificate in the chain, i.e. their leaf
// certificate.  If two chains have equal leaf certificates, they will be sorted
// by the second certificate in the chain, and so on.  By this logic, a chain
// that is a subchain of another chain beginning at the leaf of the other chain,
// will come before the other chain after sorting.
//
// Example:
//
// Before sorting:
// A -> B -> C
// D
// A -> C
// A -> B
//
// After sorting:
// A -> B
// A -> B -> C
// A -> C
// D
func (c chainSlice) Len() int { return len(c.chains) }
func (c chainSlice) Less(i, j int) bool {
	chi := c.chains[i]
	chj := c.chains[j]
	for k := 0; k < min(len(chi), len(chj)); k++ {
		if !chi[k].Equal(chj[k]) {
			return bytes.Compare(chi[k].Raw, chj[k].Raw) < 0
		}
	}
	return len(chi) < len(chj)
}
func (c chainSlice) Swap(i, j int) {
	t := c.chains[i]
	c.chains[i] = c.chains[j]
	c.chains[j] = t
}

// removeSuperChains will remove super chains from the list of chains passed to
// it.  A super chain is considered to be a chain whose first x certificates are
// included in the list somewhere else as a whole chain.  Put another way, if
// there exists a chain A in the list, and another chain B that is A with some
// additional certificates chained onto the end, B is a super chain of A
// (and A is a subchain of B).
//
// Examples:
// 1) A -> B -> C is a super chain of A -> B, and both are super chains of A.
// 2) Z -> A -> B is not a super chain of A -> B, as A -> B is not at the
//    beginning of Z -> A -> B.
// 3) Calling removeSuperChains on:
//    A -> B -> C
//    A -> C
//    A -> B
//    A -> C -> D
//    will return:
//    A -> B
//    A -> C
// 4) Calling removeSuperChains on:
//    A -> B -> C
//    A -> C
//    A -> B
//    A -> C -> D
//    A
//    will return:
//    A
func removeSuperChains(chains [][]*x509.Certificate) [][]*x509.Certificate {
	// Sort the list of chains using the sorting algorithm described above.
	// This will result in chains and their super chains being grouped together
	// in the list, with the shortest chain listed first in the group (i.e. a
	// chain, and then all its super chains - if any - listed directly after
	// that chain).
	c := chainSlice{chains: chains}
	sort.Sort(c)
	var retChains [][]*x509.Certificate
NextChain:
	// Start at the beginning of the list.
	for i := 0; i < len(c.chains); {
		// Add the chain to the list of chains to be returned.
		retChains = append(retChains, c.chains[i])
		// Step past any super chains of the chain just added to the return list,
		// without adding them to the return list.  We do not want super chains
		// of other chains in our return list.  Due to the initial sort of the
		// list, any super chains of a chain will come directly after said chain.
		for j := i + 1; j < len(c.chains); j++ {
			for k := range c.chains[i] {
				// When a chain that is not a super chain of the chain most
				// recently added to the return list is found, move to that
				// chain and start over.
				if !c.chains[i][k].Equal(c.chains[j][k]) {
					i = j
					continue NextChain
				}
			}
		}
		break
	}
	return retChains
}

func (f *Fixer) fixServer() {
	defer f.wg.Done()

	for fix := range f.toFix {
		atomic.AddUint32(&f.active, 1)
		chains, ferrs := fix.handleChain()
		f.updateCounters(chains, ferrs)
		for _, ferr := range ferrs {
			f.errors <- ferr
		}

		// If handleChain() outputs valid chains that are subchains of other
		// valid chains, (where the subchains start at the leaf)
		// e.g. A -> B -> C and A -> B -> C -> D, only forward on the shorter
		// of the chains.
		for _, chain := range removeSuperChains(chains) {
			f.chains <- chain
			atomic.AddUint32(&f.validChainsOut, 1)
		}
		atomic.AddUint32(&f.active, ^uint32(0))
	}
}

func (f *Fixer) newFixServerPool(workerCount int) {
	for i := 0; i < workerCount; i++ {
		f.wg.Add(1)
		go f.fixServer()
	}
}

func (f *Fixer) logStats() {
	t := time.NewTicker(time.Second)
	go func() {
		for _ = range t.C {
			log.Printf("fixers: %d active, %d reconstructed, "+
				"%d not reconstructed, %d fixed, %d not fixed, "+
				"%d valid chains produced, %d valid chains sent",
				f.active, f.reconstructed, f.notReconstructed,
				f.fixed, f.notFixed, f.validChainsProduced, f.validChainsOut)
		}
	}()
}

// NewFixer creates a new asynchronous fixer and starts up a pool of
// workerCount workers.  Errors are pushed to the errors channel, and fixed
// chains are pushed to the chains channel.  client is used to try to get any
// missing certificates that are needed when attempting to fix chains.
func NewFixer(workerCount int, chains chan<- []*x509.Certificate, errors chan<- *FixError, client *http.Client, logStats bool) *Fixer {
	f := &Fixer{
		toFix:  make(chan *toFix),
		chains: chains,
		errors: errors,
		cache:  newURLCache(client, logStats),
	}

	f.newFixServerPool(workerCount)
	if logStats {
		f.logStats()
	}
	return f
}
