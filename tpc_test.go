package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/garyburd/redigo/redis"
	"github.com/nlopes/slack"
	"github.com/rafaeljusto/redigomock"
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

func NewTestConfig(t *testing.T) *Config {
	tmpDir := "/tmp/tpc-test-backup"
	os.MkdirAll(tmpDir, os.ModePerm)
	tmpfile, err := ioutil.TempFile(tmpDir, "tpc-out")
	if err != nil {
		t.Fatal(err)
	}
	return &Config{
		Out:           tmpfile.Name(),
		Backup:        tmpDir,
		Cmd:           "echo mycmd",
		MasterPattern: "",
		WriteCh:       make(chan bool),
		DoneCh:        make(chan bool),

		Name:               "tpc_test",
		Ip:                 "0.0.0.0",
		Port:               9000,
		Hash:               "fnv1a_64",
		HashTag:            "",
		Distribution:       "ketama",
		Timeout:            400,
		Backlog:            512,
		RedisAuth:          "",
		RedisDb:            0,
		ClientConnections:  16384,
		ServerConnections:  1,
		Preconnect:         false,
		AutoEjectHosts:     true,
		ServerRetryTimeout: 30000,
		ServerFailureLimit: 3,

		Servers: []*Server{},
	}
}

func ExpectResult(t *testing.T, result string, expect []string) {
	for _, e := range expect {
		if !strings.Contains(result, e) {
			t.Fatalf("Expected result to contain '%s', but it did not:\n%s\n", e, result)
		}
	}
}

func ExpectResultTimes(t *testing.T, result string, sub string, times int) {
	n := strings.Count(result, sub)
	if n != times {
		t.Fatalf("Expected result to contain '%s' %d times, but was %d times\n", sub, times, n)
	}
}

func NotExpectResult(t *testing.T, result string, expect []string) {
	for _, e := range expect {
		if strings.Contains(result, e) {
			t.Fatalf("Did not expect result to contain '%s', but it did:\n%s\n", e, result)
		}
	}
}

func TestConfigAlert(t *testing.T) {
	slackClient = NewTestSlackClient()
	config := &Config{
		Name: "alert-test",
	}
	msg := "Word."
	n, err := ConfigAlert(config, msg)
	if err != nil {
		t.Error("Error sending config alert: %s\n", err)
	}

	messages := slackClient.(*TestSlackClient).Messages
	if len(messages) != 1 {
		t.Errorf("ConfigAlert: expected 1 message but got %d\n", len(messages))
	}

	myMsg := "tpc.test alert-test: Word."
	if messages[0] != myMsg {
		t.Errorf("ConfigAlert: expected message '%s' but got '%s'\n", myMsg, messages[0])
	}

	if n != len(messages[0]) {
		t.Errorf("ConfigAlert: expected message length %d but got %d\n", len(messages[0]), n)
	}
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

func TestWriteConfig(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	config.Servers = append(config.Servers,
		&Server{"ZZZ", "1.2.3.4", "8000"},
		&Server{"AAA", "1.2.3.5", "8001"})

	err := WriteConfig(config)
	if err != nil {
		t.Errorf("WriteConfig error: %s\n", err)
	}

	files, err := ioutil.ReadDir(config.Backup)
	if err != nil {
		t.Errorf("ReadDir error: %s\n", err)
	}

	expected := `tpc_test:
    listen: 0.0.0.0:9000
    hash: fnv1a_64
    distribution: ketama
    redis: true
    preconnect: false
    auto_eject_hosts: true
    server_retry_timeout: 30000
    server_failure_limit: 3
    timeout: 400
    backlog: 512
    redis_db: 0
    client_connections: 16384
    server_connections: 1
    servers:
    - 1.2.3.5:8001:1 AAA
    - 1.2.3.4:8000:1 ZZZ
`

	fileCount := 0
	for _, f := range files {
		content, err := ioutil.ReadFile(config.Backup + "/" + f.Name())
		if err != nil {
			t.Errorf("Error reading file %s: %s\n", f.Name(), err)
		}

		if string(content) != expected {
			t.Errorf("WriteConfig expected '%s' but got '%s'\n", expected, content)
		}

		fileCount += 1
	}

	if fileCount != 2 {
		t.Errorf("WriteConfig expected %d files but got %d", 2, fileCount)
	}

	suffixes := strings.Split(out.String(), "/")
	suffix := suffixes[len(suffixes)-1]
	expect := []string{
		"Writing to file: /tmp/tpc-test-backup/" + suffix,
	}

	ExpectResult(t, out.String(), expect)
}

func TestExecCmd(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	ExecCmd(config)

	expect := []string{
		"Running command: 'echo mycmd'",
		"Command output: 'mycmd'",
	}

	ExpectResult(t, out.String(), expect)
}

func TestDoConfigUpdate(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	config.Servers = append(config.Servers,
		&Server{"ZZZ", "1.2.3.4", "8000"},
		&Server{"AAA", "1.2.3.5", "8001"})

	DoConfigUpdate(config)

	expect := []string{
		"Running command: 'echo mycmd'",
		"Command output: 'mycmd'",
	}

	ExpectResult(t, out.String(), expect)
}

func TestDoConfigUpdateMulti(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	config.Servers = append(config.Servers,
		&Server{"ZZZ", "1.2.3.4", "8000"},
		&Server{"AAA", "1.2.3.5", "8001"})

	DoConfigUpdate(config)
	config.Servers[0].Ip = "1.2.3.10"
	DoConfigUpdate(config)
	config.Servers[1].Ip = "1.2.3.11"
	DoConfigUpdate(config)

	ExpectResultTimes(t, out.String(), "Doing config update", 3)
}

func TestParseSwitchMaster(t *testing.T) {
	switchMasterStr := "node-01 1.2.3.4 8000 1.2.3.5 8001"
	switchMaster, err := ParseSwitchMaster(switchMasterStr)

	if err != nil {
		t.Errorf("Error parsing SwitchMaster string: %s\n", err)
	}

	if switchMaster.MasterName != "node-01" ||
		switchMaster.OldIp != "1.2.3.4" ||
		switchMaster.OldPort != "8000" ||
		switchMaster.NewIp != "1.2.3.5" ||
		switchMaster.NewPort != "8001" {
		t.Errorf("Invalid parsing of switch master data: '%s', got %+v\n", switchMasterStr, switchMaster)
	}
}

func TestParseNonMasterInstanceDetails(t *testing.T) {
	for _, tt := range []struct {
		a string
		r *InstanceDetails
		e error
	}{
		{
			"master node-01 1.2.3.4 8000 A Description",
			&InstanceDetails{
				"master",
				"node-01",
				"1.2.3.4",
				"8000",
				"",
				"",
				"",
				"A Description",
			},
			nil,
		},
		{
			"slave node-01 1.2.3.4 8000 @ node-02 1.2.3.5 8001 A Description",
			&InstanceDetails{
				"slave",
				"node-01",
				"1.2.3.4",
				"8000",
				"node-02",
				"1.2.3.5",
				"8001",
				"A Description",
			},
			nil,
		},
		{
			"sentinel node-01 1.2.3.4 8000 @ node-02 1.2.3.5 8001 A Description",
			&InstanceDetails{
				"sentinel",
				"node-01",
				"1.2.3.4",
				"8000",
				"node-02",
				"1.2.3.5",
				"8001",
				"A Description",
			},
			nil,
		},
	} {
		r, e := ParseInstanceDetails(tt.a)
		if r.InstanceType != tt.r.InstanceType ||
			r.Name != tt.r.Name ||
			r.Ip != tt.r.Ip ||
			r.Port != tt.r.Port ||
			r.MasterName != tt.r.MasterName ||
			r.MasterIp != tt.r.MasterIp ||
			r.MasterPort != tt.r.MasterPort ||
			r.Description != tt.r.Description ||
			e != tt.e {
			t.Errorf("ParseInstanceDetails('%s') => ('%+v', '%s'), want ('%+v', '%s')\n",
				tt.a, r, e, tt.r, tt.e)
		}
	}
}

func TestHandleSwitchMaster(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	config.Servers = append(config.Servers,
		&Server{"node-01", "1.2.3.4", "8000"},
		&Server{"node-02", "1.2.3.5", "8001"})

	switchMaster := &SwitchMaster{
		"node-01", "1.2.3.4", "8000", "1.2.3.6", "8002",
	}

	go func() {
		<-config.WriteCh
	}()

	HandleSwitchMaster(switchMaster, config)

	for i, server := range []*Server{
		&Server{"node-01", "1.2.3.6", "8002"},
		&Server{"node-02", "1.2.3.5", "8001"},
	} {
		if server.Name != config.Servers[i].Name ||
			server.Ip != config.Servers[i].Ip ||
			server.Port != config.Servers[i].Port {
			t.Errorf("Expected server %d to be %+v, got %+v\n", i, server, config.Servers[i])
		}
	}

	expect := []string{
		"Replacing master node-01 1.2.3.4:8000 with 1.2.3.6:8002",
	}

	ExpectResult(t, out.String(), expect)
}

func TestHandleSwitchMasterPattern(t *testing.T) {
	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	config.MasterPattern = "node-"
	defer os.RemoveAll(config.Backup)

	config.Servers = append(config.Servers,
		&Server{"node-01", "1.2.3.4", "8000"},
		&Server{"node-02", "1.2.3.5", "8001"})

	switchMaster := &SwitchMaster{
		"instance-01", "1.3.3.1", "7000", "1.3.3.2", "7001",
	}

	go func() {
		<-config.WriteCh
	}()

	HandleSwitchMaster(switchMaster, config)

	for i, server := range []*Server{
		&Server{"node-01", "1.2.3.4", "8000"},
		&Server{"node-02", "1.2.3.5", "8001"},
	} {
		if server.Name != config.Servers[i].Name ||
			server.Ip != config.Servers[i].Ip ||
			server.Port != config.Servers[i].Port {
			t.Errorf("Expected server %d to be %+v, got %+v\n", i, server, config.Servers[i])
		}
	}

	expect := []string{
		"Ignoring switch-master for instance-01 because it does not match master_pattern",
	}

	ExpectResult(t, out.String(), expect)
}

func TestSetConfigServers(t *testing.T) {
	var values []interface{}
	values = []interface{}{
		[]interface{}{
			[]byte("ip"),
			[]byte("1.2.3.4"),
			[]byte("port"),
			[]byte("8000"),
			[]byte("name"),
			[]byte("node-01"),
		},
	}
	conn := redigomock.NewConn()
	conn.Command("SENTINEL", "masters").Expect(values)

	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	defer os.RemoveAll(config.Backup)

	err := SetConfigServers(conn, config)
	if err != nil {
		t.Errorf("Error setting config servers: %s\n", err)
	}

	server := config.Servers[0]
	if server.Ip != "1.2.3.4" ||
		server.Port != "8000" ||
		server.Name != "node-01" {
		t.Errorf("Unexpected server master: %+v\n", server)
	}
}

func TestSetConfigServersMasterPattern(t *testing.T) {
	var values []interface{}
	values = []interface{}{
		[]interface{}{
			[]byte("ip"),
			[]byte("1.2.3.4"),
			[]byte("port"),
			[]byte("8000"),
			[]byte("name"),
			[]byte("node-01"),
		},
		[]interface{}{
			[]byte("ip"),
			[]byte("4.3.2.1"),
			[]byte("port"),
			[]byte("9000"),
			[]byte("name"),
			[]byte("instance-01"),
		},
		[]interface{}{
			[]byte("ip"),
			[]byte("1.2.3.5"),
			[]byte("port"),
			[]byte("8001"),
			[]byte("name"),
			[]byte("node-02"),
		},
	}
	conn := redigomock.NewConn()
	conn.Command("SENTINEL", "masters").Expect(values)

	var out bytes.Buffer
	SetLoggerWriter(&out)

	config := NewTestConfig(t)
	config.MasterPattern = "node-"
	defer os.RemoveAll(config.Backup)

	err := SetConfigServers(conn, config)
	if err != nil {
		t.Errorf("Error setting config servers: %s\n", err)
	}

	if len(config.Servers) != 2 {
		t.Errorf("Expected 2 master servers, got %d\n", len(config.Servers))
	}

	for i, ip := range []string{"1.2.3.4", "1.2.3.5"} {
		if config.Servers[i].Ip != ip {
			t.Errorf("Expected %dth server's ip to be %s, got %s", i, ip, config.Servers[i].Ip)
		}
	}
}

func TestStrSliceContains(t *testing.T) {
	for _, tt := range []struct {
		s []string
		b string
		r bool
	}{
		{[]string{}, "one", false},
		{[]string{}, "", false},
		{[]string{"one"}, "one", true},
		{[]string{"one"}, "", false},
		{[]string{"one"}, "two", false},
		{[]string{"one", "two"}, "one", true},
		{[]string{"one", "two"}, "three", false},
	} {
		r := strSliceContains(tt.s, tt.b)
		if r != tt.r {
			t.Errorf("strSliceContains(%v, %s) => %t, want %t", tt.s, tt.b, r, tt.r)
		}
	}
}
