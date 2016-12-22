package tpc

import "testing"

func TestNodeFromMap(t *testing.T) {
	data := map[string]string{
		"ip":   "1.2.3.4",
		"port": "1234",
		"name": "node-1234",
	}
	node, err := NodeFromMap(data)
	if err != nil {
		t.Errorf("NodeFromMap returned un unexpected error: %s\n", err)
	}
	if node.Ip != "1.2.3.4" {
		t.Errorf("NodeFromMap expected Ip 1.2.3.4 but got %s\n", node.Ip)
	}
	if node.Port != "1234" {
		t.Errorf("NodeFromMap expected Ip 1234 but got %s\n", node.Port)
	}
	if node.Name != "node-1234" {
		t.Errorf("NodeFromMap expected Name node-1234 but got %s\n", node.Name)
	}
}
