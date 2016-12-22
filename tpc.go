package tpc

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

var (
	tmpl   = template.Must(template.New("config").Parse(configTmpl))
	logger *log.Logger
)

//HashTag            string
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
	server_retry_timeout: {{.ServerRetryTimeout}}
	server_failure_limit: {{.ServerFailureLimit}}
	timeout: {{.Timeout}}
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
	Out     string
	Cmd     string
	Wait    int
	Waiting bool
	Wanted  bool
	WriteCh chan bool
	DoneCh  chan bool

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
	if i.InstanceType == "slave" {
		if i.Description != "" {
			return fmt.Sprintf("slave %s %s:%s, master: %s %s:%s (%s)",
				i.Name, i.Ip, i.Port, i.MasterName, i.MasterIp, i.MasterPort, i.Description)
		} else {
			return fmt.Sprintf("slave %s %s:%s, master: %s %s:%s",
				i.Name, i.Ip, i.Port, i.MasterName, i.MasterIp, i.MasterPort)
		}
	} else {
		if i.Description != "" {
			return fmt.Sprintf("master %s %s:%s (%s)",
				i.Name, i.Ip, i.Port, i.Description)
		} else {
			return fmt.Sprintf("master %s %s:%s",
				i.Name, i.Ip, i.Port)
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

func SetLogger(l *log.Logger) {
	logger = l
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

func DoConfigUpdate(config *Config) error {
	config.Waiting = true
	config.Wanted = false

	defer func() {
		time.Sleep(time.Second * time.Duration(config.Wait))
		logger.Println("Leaving config update wait period")
		config.Waiting = false
		if config.Wanted {
			config.WriteCh <- true
		}
	}()

	var (
		newMd5sum string
		oldMd5sum string
		err       error
	)

	if _, err := os.Stat(config.Out); os.IsNotExist(err) {
		oldMd5sum = ""
	} else {
		oldMd5sum, err = FileMd5sum(config.Out)
	}
	if err != nil {
		return err
	}

	err = WriteConfig(config)
	if err != nil {
		return err
	}

	newMd5sum, err = FileMd5sum(config.Out)
	if err != nil {
		return err
	}

	if newMd5sum == oldMd5sum {
		logger.Println("Not executing command because outfile has not changed")
	} else {
		err = ExecCmd(config)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConfigWriter(config *Config) {
	for {
		select {
		case <-config.WriteCh:
			if config.Waiting {
				config.Wanted = true
				logger.Println("Not rewriting while in wait period")
			} else {
				err := DoConfigUpdate(config)
				if err != nil {
					logger.Printf("Error updating config: %s\n", err)
				}
			}
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
	newServers := []*Server{}
	changes := false
	for _, server := range config.Servers {
		var newServer *Server
		if server.Name == switchMaster.MasterName &&
			server.Ip == switchMaster.OldIp &&
			server.Port == switchMaster.OldPort {
			changes = true
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

	if !changes {
		return
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
	if len(splitServerData) != 4 ||
		(splitServerData[0] != "slave" &&
			splitServerData[0] != "sentinel") {
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

func SetConfigServers(conn redis.Conn, config *Config) error {
	config.Servers = []*Server{}

	// query sentinel for initial master configuration
	mastersData, err := redis.Values(conn.Do("SENTINEL", "masters"))
	if err != nil {
		return err
	}

	for _, masterData := range mastersData {
		masterMap, err := redis.StringMap(masterData, nil)
		if err != nil {
			return err
		}

		server, err := ServerFromMap(masterMap)
		if err != nil {
			return err
		}

		config.Servers = append(config.Servers, server)
	}

	return nil
}

func Listen(addr string, config *Config) error {
	conn, err := redis.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = SetConfigServers(conn, config)
	if err != nil {
		return err
	}

	go ConfigWriter(config)
	config.WriteCh <- true

	psc := redis.PubSubConn{conn}
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
			err = v
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

	config.DoneCh <- true

	return err
}

func ListenCluster(addrs []string, config *Config) {
	num := len(addrs)

	retriesPerServer := 3
	// try each sentinel max of 3 times in round-robin
	maxRetries := num*retriesPerServer - 1
	retries := 0

	for {
		addr := addrs[retries%retriesPerServer]
		logger.Printf("Connecting to sentinel %s\n", addr)
		err := Listen(addr, config)
		if err != nil {
			logger.Printf("Error from sentinel %s: %s\n", addr, err)
		}
		retries += 1

		if retries >= maxRetries {
			logger.Printf("Maximum retry count for all sentinel servers reached")
			break
		} else {
			time.Sleep(time.Second * 1)
		}
	}
}
