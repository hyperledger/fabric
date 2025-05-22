package bft

import (
    "testing"
    "time"
)

func TestTimeWeightedLeaderSelection(t *testing.T) {
    nodes := []*Node{
        {ID: "node1", IsActive: true, LastPing: time.Now().Add(-5 * time.Second)},
        {ID: "node2", IsActive: true, LastPing: time.Now().Add(-2 * time.Second)},
        {ID: "node3", IsActive: false, LastPing: time.Now().Add(-20 * time.Second)},
    }

    bft := &BiniBFTTimeWeighted{
        nodes: nodes,
        weights: map[string]float64{
            "node1": 0.6, // Older ping, lower weight
            "node2": 0.9, // Recent ping, higher weight
            "node3": 0.0, // Inactive, no weight
        },
    }

    leader, err := bft.SelectLeader()
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if leader == nil || leader.ID != "node2" {
        t.Errorf("expected leader node2, got %v", leader)
    }
}

func TestTimeWeightedNoActiveNodes(t *testing.T) {
    nodes := []*Node{
        {ID: "node1", IsActive: false, LastPing: time.Now().Add(-30 * time.Second)},
    }
    bft := &BiniBFTTimeWeighted{
        nodes:   nodes,
        weights: map[string]float64{"node1": 0.0},
    }

    _, err := bft.SelectLeader()
    if err == nil {
        t.Fatal("expected error for no active nodes, got nil")
    }
}
