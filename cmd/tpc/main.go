package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/garyburd/redigo/redis"
)

// flags
var (
	sentinelsFlag = flag.String("sentinels", "", "CSV of host:port to redis sentinels")
	logFlag       = flag.String("log", "", "Path to log file, will write to STDOUT if empty")
	outFlag       = flag.String("out", "", "File to write configuration, will write to STDOUT if empty")
	cmdFlag       = flag.String("cmd", "", "Command to execute after master failover")
)

var (
	tmpl   *template.Template
	logger *log.Logger
)

type Node struct {
	Name string
	Ip   string
	Port string
}

type Config struct {
	Name         string
	Ip           string
	Port         string
	Hash         string
	Distribution string
	Preconnect   bool
	Masters      []*Node
	Out          string
	Cmd          string
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
		return fmt.Sprintf("slave %s %s:%s, master: %s %s:%s (%s)",
			i.Name, i.Ip, i.Port, i.MasterName, i.MasterIp, i.MasterPort, i.Description)
	} else {
		return fmt.Sprintf("master %s %s:%s (%s)",
			i.Name, i.Ip, i.Port, i.Description)
	}
}

type SwitchMaster struct {
	MasterName string
	OldIp      string
	OldPort    string
	NewIp      string
	NewPort    string
}

const (
	configTmpl = `{{.Name}}:
	listen: {{.Ip}}:{{.Port}}
	hash: {{.Hash}}
	distribution: {{.Distribution}}
	redis: true
	preconnect: {{.Preconnect}}
	servers:
	{{ range .Masters -}}
	- {{.Ip}}:{{.Port}}:1 {{.Name}}
	{{ end -}}`
)

func NodeFromMap(nodeData map[string]string) (*Node, error) {
	node := &Node{}

	if val, ok := nodeData["ip"]; ok {
		node.Ip = val
	} else {
		return nil, errors.New("nodeData map missing 'ip' key")
	}

	if val, ok := nodeData["port"]; ok {
		node.Port = val
	} else {
		return nil, errors.New("nodeData map missing 'port' key")
	}

	if val, ok := nodeData["name"]; ok {
		node.Name = val
	} else {
		return nil, errors.New("nodeData map missing 'name' key")
	}

	return node, nil
}

func WriteConfig(config *Config) error {
	var (
		out io.Writer
		err error
	)

	if config.Out == "" {
		out = os.Stdout
	} else {
		out, err = os.OpenFile(config.Out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer out.(*os.File).Close()
	}

	err = tmpl.Execute(out, config)
	if err != nil {
		return err
	}

	return nil
}

func ExecCmd(config *Config) error {
	if config.Cmd != "" {
		logger.Printf("Running command: '%s'\n", config.Cmd)
		cmdArgs := strings.Split(config.Cmd, " ")
		cmd := cmdArgs[0]
		var args []string
		if len(cmdArgs) > 1 {
			args = cmdArgs[1:]
		} else {
			args = []string{}
		}

		_, err := exec.Command(cmd, args...).Output()
		if err != nil {
			return err
		}
	}

	return nil
}

func HandlePMessage(msg redis.PMessage, config *Config) *Config {
	switch msg.Channel {
	default:
		logger.Printf("unhandled message '%s': %s\n", msg.Channel, msg.Data)
	case "+switch-master":
		switchMaster, err := ParseSwitchMaster(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +switch-master msg: %s\n", err)
		} else {
			config = HandleSwitchMaster(switchMaster, config)
		}
	case "+slave":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +slave msg: %s\n", err)
		} else {
			config = HandlePosSlave(instanceDetails, config)
		}
	case "-role-change":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -role-change msg: %s\n", err)
		} else {
			config = HandleNegRoleChange(instanceDetails, config)
		}
	case "+role-change":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +role-change msg: %s\n", err)
		} else {
			config = HandlePosRoleChange(instanceDetails, config)
		}
	case "-sdown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -sdown msg: %s\n", err)
		} else {
			config = HandleNegSubjectivelyDown(instanceDetails, config)
		}
	case "+sdown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +sdown msg: %s\n", err)
		} else {
			config = HandlePosSubjectivelyDown(instanceDetails, config)
		}
	case "-odown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing -odown msg: %s\n", err)
		} else {
			config = HandleNegObjectivelyDown(instanceDetails, config)
		}
	case "+odown":
		instanceDetails, err := ParseInstanceDetails(string(msg.Data))
		if err != nil {
			logger.Printf("Error parsing +odown msg: %s\n", err)
		} else {
			config = HandlePosObjectivelyDown(instanceDetails, config)
		}
	}

	return config
}

func HandleSwitchMaster(switchMaster *SwitchMaster, config *Config) *Config {
	newMasters := []*Node{}
	changes := false
	for _, master := range config.Masters {
		var newMaster *Node
		if master.Name == switchMaster.MasterName &&
			master.Ip == switchMaster.OldIp &&
			master.Port == switchMaster.OldPort {
			changes = true
			newMaster = &Node{
				Name: switchMaster.MasterName,
				Ip:   switchMaster.NewIp,
				Port: switchMaster.NewPort,
			}
			logger.Printf("Replacing master %s %s:%s with %s:%s\n", switchMaster.MasterName,
				switchMaster.OldIp, switchMaster.OldPort, switchMaster.NewIp, switchMaster.NewPort)
		} else {
			newMaster = master
		}
		newMasters = append(newMasters, newMaster)
	}

	if !changes {
		return config
	}

	config.Masters = newMasters

	err := WriteConfig(config)
	if err != nil {
		logger.Printf("Error writing config: %s\n", err)
		return config
	}

	err = ExecCmd(config)
	if err != nil {
		logger.Printf("Error executing cmd: %s\n", err)
		return config
	}

	return config
}

func HandlePosSlave(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandlePosSlave:", instanceDetails)
	return config
}

func HandleNegRoleChange(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandleNegRoleChange:", instanceDetails)
	return config
}

func HandlePosRoleChange(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandlePosRoleChange:", instanceDetails)
	return config
}

func HandleNegSubjectivelyDown(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandleNegSubjectivelyDown:", instanceDetails)
	return config
}

func HandlePosSubjectivelyDown(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandlePosSubjectivelyDown:", instanceDetails)
	return config
}

func HandleNegObjectivelyDown(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandleNegObjectivelyDown:", instanceDetails)
	return config
}

func HandlePosObjectivelyDown(instanceDetails *InstanceDetails, config *Config) *Config {
	logger.Println("HandlePosObjectivelyDown:", instanceDetails)
	return config
}

func ParseInstanceDetails(data string) (*InstanceDetails, error) {
	splitData := strings.Split(data, " @ ")
	switch len(splitData) {
	default:
		return nil, fmt.Errorf("Invalid instance details: %s\n", data)
	case 1:
		return ParseMasterInstanceDetails(splitData[0])
	case 2:
		return ParseSlaveInstanceDetails(splitData[0], splitData[1])
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

func ParseSlaveInstanceDetails(slaveData, masterData string) (*InstanceDetails, error) {
	splitSlaveData := strings.Split(slaveData, " ")
	if len(splitSlaveData) != 4 || splitSlaveData[0] != "slave" {
		return nil, fmt.Errorf("Invalid slave instance details: %s\n", slaveData)
	}

	splitMasterData := strings.Split(masterData, " ")
	if len(splitMasterData) < 3 {
		return nil, fmt.Errorf("Invalid master instance details for slave: %s\n", masterData)
	}

	instanceDetails := &InstanceDetails{
		InstanceType: splitSlaveData[0],
		Name:         splitSlaveData[1],
		Ip:           splitSlaveData[2],
		Port:         splitSlaveData[3],
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

func init() {
	tmpl = template.Must(template.New("config").Parse(configTmpl))
}

func main() {
	flag.Parse()

	// setup log
	if *logFlag == "" {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	} else {
		logFile, err := os.OpenFile(*logFlag, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Printf("Error opening log file: %s\n", err)
			os.Exit(1)
		}
		defer logFile.Close()

		logger = log.New(logFile, "", log.LstdFlags)
	}

	// get sentinel configuration
	splitAddrs := strings.Split(*sentinelsFlag, ",")
	sentinelAddrs := []string{}
	for _, addr := range splitAddrs {
		if len(addr) > 0 {
			sentinelAddrs = append(sentinelAddrs, addr)
		}
	}

	if len(sentinelAddrs) == 0 {
		logger.Fatal("At least one sentinel address is required.")
	}

	// connect to first sentinel
	conn, err := redis.Dial("tcp", sentinelAddrs[0])
	if err != nil {
		logger.Fatalf("Error dialing redis sentinel: %s\n", err)
	}
	defer conn.Close()

	// query sentinel for initial master configuration
	mastersData, err := redis.Values(conn.Do("SENTINEL", "masters"))
	if err != nil {
		logger.Fatalf("Error getting list of masters: %s\n", err)
	}

	masters := make([]*Node, len(mastersData))

	for i, masterData := range mastersData {
		masterMap, err := redis.StringMap(masterData, nil)
		if err != nil {
			logger.Fatalf("Error parsing master data: %s\n", err)
		}

		master, err := NodeFromMap(masterMap)
		if err != nil {
			logger.Fatalf("Error parsing master map: %s\n", err)
		}

		masters[i] = master
	}

	config := &Config{
		Name:         "drip-persistent",
		Ip:           "0.0.0.0",
		Port:         "9000",
		Hash:         "fnv1a_64",
		Distribution: "ketama",
		Preconnect:   true,
		Masters:      masters,
		Out:          *outFlag,
		Cmd:          *cmdFlag,
	}

	err = WriteConfig(config)
	if err != nil {
		logger.Fatalf("Error writing config: %s\n", err)
	}

	psc := redis.PubSubConn{conn}
	psc.PSubscribe("*")
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			logger.Printf("%s: message: %s\n", v.Channel, v.Data)
		case redis.PMessage:
			config = HandlePMessage(v, config)
		case redis.Subscription:
			logger.Printf("%s: %s %d\n", v.Channel, v.Kind, v.Count)
		case error:
			logger.Printf("Pubsub error: %s\n", v)
		default:
			logger.Printf("Received unhandled message: %+v\n", v)
		}
	}
}
