package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/DripEmail/tpc"
)

// flags
var (
	// process flags
	sentinelsFlag = flag.String("sentinels", "", "CSV of host:port to redis sentinels")
	logFlag       = flag.String("log", "", "Path to log file, will write to STDOUT if empty")
	outFlag       = flag.String("out", "", "File to write configuration, will write to STDOUT if empty")
	cmdFlag       = flag.String("cmd", "killall -USR1 nutcracker", "Command to execute after master failover")
	waitFlag      = flag.Int("wait", 60, "Minimum number of seconds to wait between cmd execution")

	// twemproxy flags
	nameFlag               = flag.String("name", "redis", "Name of redis pool for twemproxy config")
	ipFlag                 = flag.String("ip", "0.0.0.0", "Ip address for twemproxy to bind to")
	portFlag               = flag.Int("port", 9000, "Port for twemproxy to bind to")
	hashFlag               = flag.String("hash", "fnv1a_64", "Hash algorithm for twemproxy to use")
	hashTagFlag            = flag.String("has_tag", "", "A two character string that specifies the part of the key used for hashing. Eg '{}' or '$$'")
	distributionFlag       = flag.String("distribution", "ketama", "Key distribution for twemproxy to use")
	timeoutFlag            = flag.Int("timeout", 400, "The timeout value in msec that we wait for to establish a connection to the server or receive a response from a server.")
	backlogFlag            = flag.Int("backlog", 512, "twemproxy TCP backlog argument")
	redisAuthFlag          = flag.String("redis_auth", "", "twemproxy authenticate to the redis server on connect")
	redisDbFlag            = flag.Int("redis_db", 0, "The DB number to use on the redis pool servers. Twemproxy will always present itself to clients as DB 0")
	clientConnectionsFlag  = flag.Int("client_connections", 16384, "The maximum number of connections allowed from redis clients")
	serverConnectionsFlag  = flag.Int("server_connections", 1, "The maximum number of connections that twemproxy can be open to each server")
	preconnectFlag         = flag.Bool("preconnect", false, "A boolean value that controls if twemproxy should preconnect to all the servers in this pool on process start")
	autoEjectHostsFlag     = flag.Bool("auto_eject_hosts", true, "A boolean value that controls if server should be ejected temporarily when it fails consecutively server_failure_limit times.")
	serverRetryTimeoutFlag = flag.Int("server_retry_timeout", 30000, "The timeout value in msec to wait for before retrying on a temporarily ejected server, when auto_eject_host is set to true.")
	serverFailureLimitFlag = flag.Int("server_failure_limit", 3, "The number of consecutive failures on a server that would lead to it being temporarily ejected when auto_eject_host is set to true.")
)

func main() {
	flag.Parse()

	var l *log.Logger

	// setup log
	if *logFlag == "" {
		l = log.New(os.Stdout, "", log.LstdFlags)
	} else {
		logFile, err := os.OpenFile(*logFlag, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Printf("Error opening log file: %s\n", err)
			os.Exit(1)
		}
		defer logFile.Close()

		l = log.New(logFile, "", log.LstdFlags)
	}

	tpc.SetLogger(l)

	// get sentinel configuration
	splitAddrs := strings.Split(*sentinelsFlag, ",")
	sentinelAddrs := []string{}
	for _, addr := range splitAddrs {
		if len(addr) > 0 {
			sentinelAddrs = append(sentinelAddrs, addr)
		}
	}

	if len(sentinelAddrs) == 0 {
		l.Fatal("At least one sentinel address is required.")
	}

	config := tpc.Config{
		Out:     *outFlag,
		Cmd:     *cmdFlag,
		Wait:    *waitFlag,
		Waiting: false,
		Wanted:  false,
		WriteCh: make(chan bool),
		DoneCh:  make(chan bool),

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

	tpc.ListenCluster(sentinelAddrs, &config)
	l.Println("Goodbye!")
}
