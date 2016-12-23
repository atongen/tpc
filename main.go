package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// flags
var (
	// process flags
	sentinelsFlag     = flag.String("sentinels", "", "CSV of host:port to redis sentinels")
	logFlag           = flag.String("log", "", "Path to log file, will write to STDOUT if empty")
	outFlag           = flag.String("out", "", "File to write configuration, will write to STDOUT if empty")
	cmdFlag           = flag.String("cmd", "killall -USR1 nutcracker", "Command to execute after master failover")
	waitFlag          = flag.Int("wait", 60, "Minimum number of seconds to wait between cmd execution")
	masterPatternFlag = flag.String("master_pattern", "", "If provided, will filter master names from sentinel based on pattern")

	tokenFlag     = flag.String("token", "", "Slack: API token used for notifications")
	channelFlag   = flag.String("channel", "#incidents", "Slack: channel for notifications")
	usernameFlag  = flag.String("username", "", "Slack: username for notifications")
	iconEmojiFlag = flag.String("icon_emoji", "", "Slack: icon emoji for notifications")

	// twemproxy flags
	nameFlag               = flag.String("name", "redis", "Twemproxy: Name of redis pool")
	ipFlag                 = flag.String("ip", "0.0.0.0", "Twemproxy: Ip address")
	portFlag               = flag.Int("port", 9000, "Twemproxy: Port")
	hashFlag               = flag.String("hash", "fnv1a_64", "Twemproxy: Hash algorithm")
	hashTagFlag            = flag.String("has_tag", "", "Twemproxy: A two character string that specifies the part of the key used for hashing. Eg '{}' or '$$'")
	distributionFlag       = flag.String("distribution", "ketama", "Twemproxy: Key distribution")
	timeoutFlag            = flag.Int("timeout", 400, "Twemproxy: The timeout value in msec that we wait for to establish a connection to the server or receive a response from a server.")
	backlogFlag            = flag.Int("backlog", 512, "Twemproxy: TCP backlog argument")
	redisAuthFlag          = flag.String("redis_auth", "", "Twemproxy: authenticate to the redis server on connect")
	redisDbFlag            = flag.Int("redis_db", 0, "Twemproxy: The DB number to use on the redis pool servers. Twemproxy will always present itself to clients as DB 0")
	clientConnectionsFlag  = flag.Int("client_connections", 16384, "Twemproxy: The maximum number of connections allowed from redis clients")
	serverConnectionsFlag  = flag.Int("server_connections", 1, "Twemproxy: The maximum number of connections that can be open to each server")
	preconnectFlag         = flag.Bool("preconnect", false, "Twemproxy: A boolean value that controls if we should preconnect to all the servers in this pool on process start")
	autoEjectHostsFlag     = flag.Bool("auto_eject_hosts", true, "Twemproxy: A boolean value that controls if server should be ejected temporarily when it fails consecutively server_failure_limit times.")
	serverRetryTimeoutFlag = flag.Int("server_retry_timeout", 30000, "Twemproxy: The timeout value in msec to wait for before retrying on a temporarily ejected server, when auto_eject_host is set to true.")
	serverFailureLimitFlag = flag.Int("server_failure_limit", 3, "Twemproxy: The number of consecutive failures on a server that would lead to it being temporarily ejected when auto_eject_host is set to true.")
)

func main() {
	flag.Parse()

	err := SetLogger(*logFlag)
	if err != nil {
		fmt.Printf("Error opening log file: %s\n", err)
		os.Exit(1)
	}

	err = SetSlack(*tokenFlag, *channelFlag, *usernameFlag, *iconEmojiFlag)
	if err != nil {
		fmt.Printf("Error setting up slack client: %s\n", err)
		os.Exit(1)
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
		fmt.Println("At least one sentinel address is required.")
		os.Exit(1)
	}

	config := Config{
		Out:           *outFlag,
		Cmd:           *cmdFlag,
		MasterPattern: *masterPatternFlag,
		Wait:          *waitFlag,
		Waiting:       false,
		Wanted:        false,
		WriteCh:       make(chan bool),
		DoneCh:        make(chan bool),

		Name:               *nameFlag,
		Ip:                 *ipFlag,
		Port:               *portFlag,
		Hash:               *hashFlag,
		HashTag:            *hashTagFlag,
		Distribution:       *distributionFlag,
		Timeout:            *timeoutFlag,
		Backlog:            *backlogFlag,
		RedisAuth:          *redisAuthFlag,
		RedisDb:            *redisDbFlag,
		ClientConnections:  *clientConnectionsFlag,
		ServerConnections:  *serverConnectionsFlag,
		Preconnect:         *preconnectFlag,
		AutoEjectHosts:     *autoEjectHostsFlag,
		ServerRetryTimeout: *serverRetryTimeoutFlag,
		ServerFailureLimit: *serverFailureLimitFlag,
	}

	ListenCluster(sentinelAddrs, &config)
}
