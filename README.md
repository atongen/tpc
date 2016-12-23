# tpc

tpc roughtly stands for Twemproxy Configurator

Listens to a cluster of redis sentinel servers via pubsub.
Rewrites twemproxy config file and executes twemproxy reload command when
sentinel notifies of redis master failover.

## Install

```sh
$ go get github.com/DripEmail/tpc
```

or

```sh
$ mkdir -p $GOPATH/src/github.com/DripEmail
$ cd $GOPATH/src/github.com/DripEmail
$ git clone tpc
$ cd tpc
$ go install
$ rehash
```

## Testing

```sh
$ cd $GOPATH/src/github.com/DripEmail/tpc
$ go test
```

## Example

Here's an example of the output of running tpc, then killing a redis master process:

```
$ tpc -sentinels :8000,:8001,:8002,:8003,:8004 -out /home/atongen/Workspace/drip/nutcracker/nutcracker.yml -wait 10
2016/12/23 12:49:56 Connecting to sentinel :8000
2016/12/23 12:49:56 Writing to outfile: /home/atongen/Workspace/drip/nutcracker/nutcracker.yml
2016/12/23 12:49:56 *: psubscribe 1
2016/12/23 12:49:56 Running command: 'killall -USR1 nutcracker'
2016/12/23 12:50:06 Leaving config update wait period
2016/12/23 12:50:06 Error updating config: exit status 1
2016/12/23 12:51:06 HandlePosSubjectivelyDown: master node-25 127.0.0.1:6025
2016/12/23 12:51:06 unhandled message '+new-epoch': 1
2016/12/23 12:51:06 unhandled message '+vote-for-leader': 48982c549e62dd5f5c4e2b27254af2b9c33f60be 1
2016/12/23 12:51:06 HandlePosObjectivelyDown: master node-25 127.0.0.1:6025 (#quorum 5/3)
2016/12/23 12:51:07 HandleNegRoleChange: slave 127.0.0.1:7025 127.0.0.1:7025, master: node-25 127.0.0.1:6025 (new reported role is master)
2016/12/23 12:51:08 unhandled message '+config-update-from': sentinel 48982c549e62dd5f5c4e2b27254af2b9c33f60be 127.0.0.1 8001 @ node-25 127.0.0.1 6025
2016/12/23 12:51:08 Replacing master node-25 127.0.0.1:6025 with 127.0.0.1:7025
2016/12/23 12:51:08 HandlePosSlave: slave 127.0.0.1:6025 127.0.0.1:6025, master: node-25 127.0.0.1:7025
2016/12/23 12:51:08 Writing to outfile: /home/atongen/Workspace/drip/nutcracker/nutcracker.yml
2016/12/23 12:51:08 Running command: 'killall -USR1 nutcracker'
2016/12/23 12:51:13 HandlePosSubjectivelyDown: slave 127.0.0.1:6025 127.0.0.1:6025, master: node-25 127.0.0.1:7025
2016/12/23 12:51:18 Leaving config update wait period
```

## Command-Line Help

```
$ tpc -h
Usage of tpc:
  -auto_eject_hosts
        Twemproxy: A boolean value that controls if server should be ejected temporarily when it fails consecutively server_failure_limit times. (default true)
  -backlog int
        Twemproxy: TCP backlog argument (default 512)
  -channel string
        Slack: channel for notifications (default "#incidents")
  -client_connections int
        Twemproxy: The maximum number of connections allowed from redis clients (default 16384)
  -cmd string
        Command to execute after master failover (default "killall -USR1 nutcracker")
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
        Twemproxy: A boolean value that controls if we should preconnect to all the servers in this pool on process start
  -redis_auth string
        Twemproxy: authenticate to the redis server on connect
  -redis_db int
        Twemproxy: The DB number to use on the redis pool servers. Twemproxy will always present itself to clients as DB 0
  -sentinels string
        CSV of host:port to redis sentinels
  -server_connections int
        Twemproxy: The maximum number of connections that can be open to each server (default 1)
  -server_failure_limit int
        Twemproxy: The number of consecutive failures on a server that would lead to it being temporarily ejected when auto_eject_host is set to true. (default 3)
  -server_retry_timeout int
        Twemproxy: The timeout value in msec to wait for before retrying on a temporarily ejected server, when auto_eject_host is set to true. (default 30000)
  -timeout int
        Twemproxy: The timeout value in msec that we wait for to establish a connection to the server or receive a response from a server. (default 400)
  -token string
        Slack: API token used for notifications
  -username string
        Slack: username for notifications
  -wait int
        Minimum number of seconds to wait between cmd execution (default 60)
```

## Issues

### twemproxy hot reload

* https://github.com/twitter/twemproxy/issues/6
* https://github.com/twitter/twemproxy/pull/321
* https://github.com/machinezone/twemproxy/tree/lwalkin/config-reload
