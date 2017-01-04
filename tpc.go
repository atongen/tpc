package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/nlopes/slack"
)

var (
	tmpl           = template.Must(template.New("config").Parse(configTmpl))
	logger         *log.Logger
	slackClient    SlackClient
	slackChannel   string
	slackUsername  string
	slackIconEmoji string
)

const (
	configTmpl = `{{.Name}}:
	listen: {{.Ip}}:{{.Port}}
	hash: {{.Hash}}
	{{ if .HashTag -}}
	hash_tag: {{.HashTag}}
	{{ end -}}
	{{ if .RedisAuth -}}
	redis_auth: {{.RedisAuth}}
	{{ end -}}
	distribution: {{.Distribution}}
	redis: true
	preconnect: {{.Preconnect}}
	auto_eject_hosts: {{.AutoEjectHosts}}
	{{ if gt .ServerRetryTimeout -1 -}}
	server_retry_timeout: {{.ServerRetryTimeout}}
	{{ end -}}
	{{ if gt .ServerFailureLimit -1 -}}
	server_failure_limit: {{.ServerFailureLimit}}
	{{ end -}}
	{{ if gt .Timeout -1 -}}
	timeout: {{.Timeout}}
	{{ end -}}
	backlog: {{.Backlog}}
	redis_db: {{.RedisDb}}
	client_connections: {{.ClientConnections}}
	server_connections: {{.ServerConnections}}
	servers:
	{{ range .Servers -}}
	- {{.Ip}}:{{.Port}}:1 {{.Name}}
	{{ end -}}`
)

type Server struct {
	Name string
	Ip   string
	Port string
}

type Servers []*Server

func (s Servers) Len() int {
	return len(s)
}

func (s Servers) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s Servers) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type Config struct {
	Out           string
	Cmd           string
	MasterPattern string
	Wait          int
	Waiting       bool
	WriteCh       chan bool
	DoneCh        chan bool

	Name               string
	Ip                 string
	Port               int
	Hash               string
	HashTag            string
	Distribution       string
	Timeout            int
	Backlog            int
	RedisAuth          string
	RedisDb            int
	ClientConnections  int
	ServerConnections  int
	Preconnect         bool
	AutoEjectHosts     bool
	ServerRetryTimeout int
	ServerFailureLimit int
	Servers            Servers
}

type InstanceDetails struct {
	InstanceType string
	Name         string
	Ip           string
	Port         string
	MasterName   string
	MasterIp     string
	MasterPort   string
	Description  string
}

func (i *InstanceDetails) String() string {
	if i.InstanceType == "master" {
		if i.Description != "" {
			return fmt.Sprintf("master %s %s:%s (%s)",
				i.Name, i.Ip, i.Port, i.Description)
		} else {
			return fmt.Sprintf("master %s %s:%s",
				i.Name, i.Ip, i.Port)
		}
	} else {
		if i.Description != "" {
			return fmt.Sprintf("%s %s %s:%s, master: %s %s:%s (%s)",
				i.InstanceType, i.Name, i.Ip, i.Port, i.MasterName, i.MasterIp, i.MasterPort, i.Description)
		} else {
			return fmt.Sprintf("%s %s %s:%s, master: %s %s:%s",
				i.InstanceType, i.Name, i.Ip, i.Port, i.MasterName, i.MasterIp, i.MasterPort)
		}
	}
}

type SwitchMaster struct {
	MasterName string
	OldIp      string
	OldPort    string
	NewIp      string
	NewPort    string
}

func (s *SwitchMaster) String() string {
	return fmt.Sprintf("switch-master: %s, old: %s:%s, new: %s:%s",
		s.MasterName, s.OldIp, s.OldPort, s.NewIp, s.NewPort)
}

// Implement just the portion of the slack client api that we using
// so we can mock this in tests
type SlackClient interface {
	AuthTest() (*slack.AuthTestResponse, error)
	PostMessage(string, string, slack.PostMessageParameters) (string, string, error)
}

func SetLogger(logPath string) error {
	// setup log
	if logPath == "" {
		SetLoggerWriter(os.Stdout)
	} else {
		logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		SetLoggerWriter(logFile)
	}
	return nil
}

func SetLoggerWriter(w io.Writer) {
	logger = log.New(w, "", log.LstdFlags)
}

func SetSlack(token, channel, username, iconEmoji string) error {
	if token == "" {
		return nil
	}

	slack.SetLogger(logger)
	myClient := slack.New(token)
	_, err := myClient.AuthTest()
	if err != nil {
		return err
	}

	slackClient = myClient

	slackChannel = channel
	slackUsername = username
	slackIconEmoji = iconEmoji

	return err
}

func Alertf(format string, a ...interface{}) (int, error) {
	if slackClient != nil {
		msg := fmt.Sprintf(format, a...)
		params := slack.NewPostMessageParameters()
		if slackUsername != "" {
			params.Username = slackUsername
		}
		if slackIconEmoji != "" {
			params.IconEmoji = slackIconEmoji
		}
		_, _, err := slackClient.PostMessage(slackChannel, msg, params)
		if err != nil {
			logger.Printf("Error sending slack alert: %s", err)
			return 0, err
		}
		return len(msg), nil
	}

	return 0, nil
}

func ConfigAlert(config *Config, msg string) (int, error) {
	appName := path.Base(os.Args[0])
	return Alertf("%s %s: %s", appName, config.Name, msg)
}

func PMessageAlert(config *Config, msg redis.PMessage) (int, error) {
	return ConfigAlert(config, fmt.Sprintf("%s %s", msg.Channel, msg.Data))
}

func ServerFromMap(serverData map[string]string) (*Server, error) {
	server := &Server{}

	if val, ok := serverData["ip"]; ok {
		server.Ip = val
	} else {
		return nil, errors.New("serverData map missing 'ip' key")
	}

	if val, ok := serverData["port"]; ok {
		server.Port = val
	} else {
		return nil, errors.New("serverData map missing 'port' key")
	}

	if val, ok := serverData["name"]; ok {
		server.Name = val
	} else {
		return nil, errors.New("serverData map missing 'name' key")
	}

	return server, nil
}

func WriteConfig(config *Config) error {
	var (
		out io.Writer
		err error
	)

	if config.Out == "" {
		out = os.Stdout
	} else {
		logger.Printf("Writing to outfile: %s\n", config.Out)
		out, err = os.OpenFile(config.Out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer out.(*os.File).Close()
	}

	sort.Sort(config.Servers)

	err = tmpl.Execute(out, config)
	if err != nil {
		return err
	}

	return CleanConfig(config)
}

func CleanConfig(config *Config) error {
	// remove trailing whitespace
	_, err := exec.Command("sed", "-i", "s/[ \t]*$//", config.Out).Output()
	if err != nil {
		return err
	}

	// expand tabs
	_, err = exec.Command("sed", "-i", "s/\t/    /g", config.Out).Output()
	if err != nil {
		return err
	}

	return nil
}

func ExecCmd(config *Config) error {
	if config.Cmd == "" {
		return nil
	}

	logger.Printf("Running command: '%s'\n", config.Cmd)
	cmdArgs := strings.Split(config.Cmd, " ")
	cmd := cmdArgs[0]
	var args []string
	if len(cmdArgs) > 1 {
		args = cmdArgs[1:]
	} else {
		args = []string{}
	}

	output, err := exec.Command(cmd, args...).Output()
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed != "" {
		logger.Printf("Command output: '%s'\n", trimmed)
	}

	return nil
}

func DoConfigUpdate(config *Config) {
	if config.Waiting {
		return
	}

	config.Waiting = true
	defer func() {
		config.Waiting = false
	}()

	logger.Println("Beginning config update wait period")
	time.Sleep(time.Second * time.Duration(config.Wait))

	err := WriteConfig(config)
	if err != nil {
		logger.Printf("Error writing config: '%s'\n", err)
		return
	}

	err = ExecCmd(config)
	if err != nil {
		logger.Printf("Error executing command: '%s'\n", err)
	}
}

func ConfigWriter(config *Config) {
	for {
		select {
		case <-config.WriteCh:
			go DoConfigUpdate(config)
		case <-config.DoneCh:
			break
		}
	}
}

func HandlePMessage(msg redis.PMessage, config *Config) {
	switch msg.Channel {
	default:
		logger.Printf("unhandled message '%s': %s\n", msg.Channel, msg.Data)
	case "+switch-master":
		PMessageAlert(config, msg)
		switchMaster, err := ParseSwitchMaster(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +switch-master msg: %s\n", err)
		} else {
			HandleSwitchMaster(switchMaster, config)
		}
	case "+slave":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +slave msg: %s\n", err)
		} else {
			HandlePosSlave(instanceDetails, config)
		}
	case "-role-change":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -role-change msg: %s\n", err)
		} else {
			HandleNegRoleChange(instanceDetails, config)
		}
	case "+role-change":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +role-change msg: %s\n", err)
		} else {
			HandlePosRoleChange(instanceDetails, config)
		}
	case "-sdown":
		PMessageAlert(config, msg)
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -sdown msg: %s\n", err)
		} else {
			HandleNegSubjectivelyDown(instanceDetails, config)
		}
	case "+sdown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +sdown msg: %s\n", err)
		} else {
			HandlePosSubjectivelyDown(instanceDetails, config)
		}
	case "-odown":
		PMessageAlert(config, msg)
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -odown msg: %s\n", err)
		} else {
			HandleNegObjectivelyDown(instanceDetails, config)
		}
	case "+odown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +odown msg: %s\n", err)
		} else {
			HandlePosObjectivelyDown(instanceDetails, config)
		}
	}
}

func HandleSwitchMaster(switchMaster *SwitchMaster, config *Config) {
	if config.MasterPattern != "" && !strings.Contains(switchMaster.MasterName, config.MasterPattern) {
		logger.Printf("Ignoring switch-master for %s because it does not match master_pattern",
			switchMaster.MasterName)
		return
	}

	newServers := []*Server{}
	for _, server := range config.Servers {
		var newServer *Server
		if server.Name == switchMaster.MasterName &&
			server.Ip == switchMaster.OldIp &&
			server.Port == switchMaster.OldPort {
			newServer = &Server{
				Name: switchMaster.MasterName,
				Ip:   switchMaster.NewIp,
				Port: switchMaster.NewPort,
			}
			logger.Printf("Replacing master %s %s:%s with %s:%s\n", switchMaster.MasterName,
				switchMaster.OldIp, switchMaster.OldPort, switchMaster.NewIp, switchMaster.NewPort)
		} else {
			newServer = server
		}
		newServers = append(newServers, newServer)
	}

	config.Servers = newServers
	config.WriteCh <- true
}

func HandlePosSlave(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandlePosSlave:", instanceDetails)
}

func HandleNegRoleChange(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandleNegRoleChange:", instanceDetails)
}

func HandlePosRoleChange(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandlePosRoleChange:", instanceDetails)
}

func HandleNegSubjectivelyDown(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandleNegSubjectivelyDown:", instanceDetails)
}

func HandlePosSubjectivelyDown(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandlePosSubjectivelyDown:", instanceDetails)
}

func HandleNegObjectivelyDown(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandleNegObjectivelyDown:", instanceDetails)
}

func HandlePosObjectivelyDown(instanceDetails *InstanceDetails, config *Config) {
	logger.Println("HandlePosObjectivelyDown:", instanceDetails)
}

func ParseInstanceDetails(data string) (*InstanceDetails, error) {
	splitData := strings.Split(data, " @ ")
	switch len(splitData) {
	default:
		return nil, fmt.Errorf("Invalid instance details: %s\n", data)
	case 1:
		return ParseMasterInstanceDetails(splitData[0])
	case 2:
		return ParseNonMasterInstanceDetails(splitData[0], splitData[1])
	}
}

func ParseMasterInstanceDetails(data string) (*InstanceDetails, error) {
	splitData := strings.Split(data, " ")
	if len(splitData) < 4 || splitData[0] != "master" {
		return nil, fmt.Errorf("Invalid master instance details: %s\n", data)
	}

	instanceDetails := &InstanceDetails{
		InstanceType: splitData[0],
		Name:         splitData[1],
		Ip:           splitData[2],
		Port:         splitData[3],
	}

	if len(splitData) > 4 {
		instanceDetails.Description = strings.Join(splitData[4:], " ")
	}

	return instanceDetails, nil
}

func ParseNonMasterInstanceDetails(serverData, masterData string) (*InstanceDetails, error) {
	splitServerData := strings.Split(serverData, " ")
	if len(splitServerData) != 4 || splitServerData[0] == "master" {
		return nil, fmt.Errorf("Invalid server instance details: %s\n", serverData)
	}

	splitMasterData := strings.Split(masterData, " ")
	if len(splitMasterData) < 3 {
		return nil, fmt.Errorf("Invalid master instance details for non-master: %s\n", masterData)
	}

	instanceDetails := &InstanceDetails{
		InstanceType: splitServerData[0],
		Name:         splitServerData[1],
		Ip:           splitServerData[2],
		Port:         splitServerData[3],
		MasterName:   splitMasterData[0],
		MasterIp:     splitMasterData[1],
		MasterPort:   splitMasterData[2],
	}

	if len(splitMasterData) > 3 {
		instanceDetails.Description = strings.Join(splitMasterData[3:], " ")
	}

	return instanceDetails, nil
}

func ParseSwitchMaster(data string) (*SwitchMaster, error) {
	splitData := strings.Split(data, " ")
	if len(splitData) != 5 {
		return nil, fmt.Errorf("Invalid switch master: %s\n", data)
	}

	return &SwitchMaster{
		MasterName: splitData[0],
		OldIp:      splitData[1],
		OldPort:    splitData[2],
		NewIp:      splitData[3],
		NewPort:    splitData[4],
	}, nil
}

func MasterServers(conn redis.Conn, filter string) (Servers, error) {
	servers := []*Server{}

	// query sentinel for initial master configuration
	mastersData, err := redis.Values(conn.Do("SENTINEL", "masters"))
	if err != nil {
		return servers, fmt.Errorf("Error parsing sentinel masters: %s\n", err)
	}

	for _, masterData := range mastersData {
		masterMap, err := redis.StringMap(masterData, nil)
		if err != nil {
			return servers, fmt.Errorf("Error parsing master data: %s\n", err)
		}

		server, err := ServerFromMap(masterMap)
		if err != nil {
			return servers, fmt.Errorf("Error parsing parsing server from master map: %s\n", err)
		}

		if filter == "" || (strings.Contains(server.Name, filter)) {
			servers = append(servers, server)
		}
	}

	return servers, nil
}

func SetConfigServers(conn redis.Conn, config *Config) error {
	servers, err := MasterServers(conn, config.MasterPattern)
	if err != nil {
		return err
	}

	config.Servers = servers
	return nil
}

func ListenSentinel(addr string, config *Config) error {
	var (
		conn redis.Conn
		err  error
	)
	conn, err = redis.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = SetConfigServers(conn, config)
	if err != nil {
		return err
	}

	config.WriteCh <- true

	psc := redis.PubSubConn{conn}
	defer psc.Close()

	psc.PSubscribe("*")
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			logger.Printf("%s: message: %s\n", v.Channel, v.Data)
		case redis.PMessage:
			HandlePMessage(v, config)
		case redis.Subscription:
			logger.Printf("%s: %s %d\n", v.Channel, v.Kind, v.Count)
		case error:
			logger.Printf("Error from sentinel pubsub: %s\n", v)
			break
		default:
			if conn.Err() != nil {
				err = conn.Err()
				break
			}
			logger.Printf("Received unhandled message: %+v\n", v)
		}
		if conn.Err() != nil {
			err = conn.Err()
			break
		}
	}

	return err
}

func ListenCluster(addrs []string, config *Config) {
	defer close(config.WriteCh)
	defer close(config.DoneCh)
	num := len(addrs)

	go ConfigWriter(config)

	retriesPerServer := 3
	// try each sentinel max of 3 times in round-robin
	maxRetries := num * retriesPerServer
	retries := 0

	for {
		if retries >= maxRetries {
			logger.Printf("Maximum retry count for all sentinel servers reached")
			break
		}

		addr := addrs[retries%num]
		logger.Printf("Connecting to sentinel %s\n", addr)

		err := ListenSentinel(addr, config)
		if err != nil {
			msg := fmt.Sprintf("Sentinel (%s) error: %s", addr, err.Error())
			logger.Println(msg)
			ConfigAlert(config, msg)
		}
		retries += 1
	}

	config.DoneCh <- true
	logger.Println("Goodbye!")
}
