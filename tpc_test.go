package main

import (
	"testing"

	"github.com/garyburd/redigo/redis"
	"github.com/nlopes/slack"
)

type TestSlackClient struct {
	Messages []string
}

func (c *TestSlackClient) AuthTest() (*slack.AuthTestResponse, error) {
	return nil, nil
}

func (c *TestSlackClient) PostMessage(channel, text string, params slack.PostMessageParameters) (string, string, error) {
	c.Messages = append(c.Messages, text)
	return channel, "timestamp", nil
}

func NewTestSlackClient() SlackClient {
	return &TestSlackClient{make([]string, 0)}
}

func TestPMessageAlert(t *testing.T) {
	slackClient = NewTestSlackClient()
	config := &Config{
		Name: "alert-test",
	}
	msg := redis.PMessage{
		Channel: "test-channel",
		Data:    []byte("test-data"),
	}
	n, err := PMessageAlert(config, msg)
	if err != nil {
		t.Error("Error sending pmessage alert: %s\n", err)
	}

	messages := slackClient.(*TestSlackClient).Messages
	if len(messages) != 1 {
		t.Errorf("PMessageAlert: expected 1 message but got %d\n", len(messages))
	}

	myMsg := "tpc.test alert-test: test-channel test-data"
	if messages[0] != myMsg {
		t.Errorf("PMessageAlert: expected message '%s' but got '%s'\n", myMsg, messages[0])
	}

	if n != len(messages[0]) {
		t.Errorf("PMessageAlert: expected message length %d but got %d\n", len(messages[0]), n)
	}
}

func TestServerFromMap(t *testing.T) {
	data := map[string]string{
		"ip":   "1.2.3.4",
		"port": "1234",
		"name": "node-1234",
	}
	node, err := ServerFromMap(data)
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
