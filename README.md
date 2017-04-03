# tpc

tpc roughtly stands for Twemproxy Configurator

Listens to a cluster of redis sentinel servers via pubsub.
Rewrites twemproxy config file and executes twemproxy reload command when
sentinel notifies of redis master failover.

## Install

Download the [latest release](https://github.com/DripEmail/tpc/releases), extract it,
and put it somewhere on your PATH.

or

```sh
$ go get github.com/DripEmail/tpc
```

or

```sh
$ mkdir -p $GOPATH/src/github.com/DripEmail
$ cd $GOPATH/src/github.com/DripEmail
$ git clone git@github.com:DripEmail/tpc.git
$ cd tpc
$ go install
$ rehash
```

## Testing

```sh
$ cd $GOPATH/src/github.com/DripEmail/tpc
$ go test -cover
```

## Releases

```sh
$ mkdir -p $GOPATH/src/github.com/DripEmail
$ cd $GOPATH/src/github.com/DripEmail
$ git clone git@github.com:DripEmail/tpc.git
$ cd tpc
$ make release
```

## Command-Line Options

```
$ tpc -h
Usage of tpc:
  -auto_eject_hosts
        Twemproxy: A boolean value that controls if server should be ejected temporarily when it fails consecutively server_failure_limit times.
  -backlog int
        Twemproxy: TCP backlog argument (default 1024)
  -channel string
        Slack: channel for notifications (default "#incidents")
  -client_connections int
        Twemproxy: The maximum number of connections allowed from redis clients (default 4096)
  -cmd string
        Command to execute after master failover
  -distribution string
        Twemproxy: Key distribution (default "ketama")
  -has_tag string
        Twemproxy: A two character string that specifies the part of the key used for hashing. Eg '{}' or '$$'
  -hash string
        Twemproxy: Hash algorithm (default "fnv1a_64")
  -icon_emoji string
        Slack: icon emoji for notifications
  -ip string
        Twemproxy: Ip address (default "0.0.0.0")
  -log string
        Path to log file, will write to STDOUT if empty
  -master_pattern string
        If provided, will filter master names from sentinel based on pattern
  -name string
        Twemproxy: Name of redis pool (default "redis")
  -out string
        File to write configuration, will write to STDOUT if empty
  -port int
        Twemproxy: Port (default 9000)
  -preconnect
        Twemproxy: A boolean value that controls if we should preconnect to all the servers in this pool on process start (default true)
  -redis_auth string
        Twemproxy: authenticate to the redis server on connect
  -redis_db int
        Twemproxy: The DB number to use on the redis pool servers. Twemproxy will always present itself to clients as DB 0
  -sentinels string
        CSV of host:port to redis sentinels
  -server_connections int
        Twemproxy: The maximum number of connections that can be open to each server (default 1)
  -server_failure_limit int
        Twemproxy: The number of consecutive failures on a server that would lead to it being temporarily ejected when auto_eject_host is set to true. (default -1)
  -server_retry_timeout int
        Twemproxy: The timeout value in msec to wait for before retrying on a temporarily ejected server, when auto_eject_host is set to true. (default -1)
  -timeout int
        Twemproxy: The timeout value in msec that we wait for to establish a connection to the server or receive a response from a server. (default -1)
  -token string
        Slack: API token used for notifications
  -username string
        Slack: username for notifications
  -v    Print version information and exit
```

## Issues

### twemproxy hot reload

There is a long-running twemproxy feature branch that is supposed to bring config hot-reload via unix signal:

* https://github.com/twitter/twemproxy/issues/6
* https://github.com/twitter/twemproxy/pull/321
* https://github.com/machinezone/twemproxy/tree/lwalkin/config-reload

Currently the solution is to pause the redis clients, then stop and restart the twemproxy process.
