package defaultImpl

import (
	"fmt"
	"testing"
)

func TestClient(t *testing.T) {
	fmt.Println("TestClientEnroll")
	c := NewClient()
	c.Enroll("testUser", "secret", "http://localhost:8888", "csr.json")
}
