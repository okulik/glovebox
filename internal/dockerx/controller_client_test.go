package dockerx

import "testing"

func TestNewControllerClient_AcceptsTCPHost(t *testing.T) {
	c, err := NewControllerClient("tcp://nowhere:2375")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil client")
	}
}
