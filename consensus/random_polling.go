package bft

import (
    "crypto/rand"
    "math/big"
    "sync"
    "time"
)

// Node represents a Fabric orderer node in the BiniBFT network.
type Node struct {
    ID        string
    IsActive  bool
    LastPing  time.Time
}

// BiniBFTRandomPolling manages the Random Polling consensus.
type BiniBFTRandomPolling struct {
    nodes      []*Node
    currentLeader *Node
    mu         sync.Mutex
}

// NewBiniBFTRandomPolling initializes the Random Polling consensus.
func NewBiniBFTRandomPolling(nodes []*Node) *BiniBFTRandomPolling {
    return &BiniBFTRandomPolling{
        nodes: nodes,
    }
}

// SelectLeader randomly selects an active node as leader.
func (b *BiniBFTRandomPolling) SelectLeader() (*Node, error) {
    b.mu.Lock()
    defer b.mu.Unlock()

    // Filter active nodes (pinged within last 10 seconds).
    activeNodes := []*Node{}
    for _, node := range b.nodes {
        if node.IsActive && time.Since(node.LastPing) < 10*time.Second {
            activeNodes = append(activeNodes, node)
        }
    }

    if len(activeNodes) == 0 {
        return nil, errors.New("no active nodes available")
    }

    // Randomly select a leader.
    index, err := rand.Int(rand.Reader, big.NewInt(int64(len(activeNodes))))
    if err != nil {
        return nil, err
    }
    b.currentLeader = activeNodes[index]
    return b.currentLeader, nil
}

// Heartbeat updates node activity status.
func (b *BiniBFTRandomPolling) Heartbeat(nodeID string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for _, node := range b.nodes {
        if node.ID == nodeID {
            node.LastPing = time.Now()
            node.IsActive = true
            break
        }
    }
}
