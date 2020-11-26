# aws-fail2ban
fail2ban for load-balanced services backed by clusters (eg AWS ECS or EKS) using AWS WAF and ip sets

Each constituent container notifies this service (via an http endpoint) of individual infractions and this service determines whether and how long to effect a ban against the offending ip.  It would be preferable if the containers could individually determine when to effect bans but being behind a load balancer means infractions would be distributed across the containers.  Which implies using a centralized manager (this approach) or extensive intra-cluster communication for sharing state.

This project itself can be implement as a service (ie as several containers) for services that handle massive amounts of connections and therefore high rates of potential bans.  The service-based implementation uses Redis to share state amongst the containers, the standalone version simply maintains state in memory.  Be aware that due to the optimistic locking provided by the `aws-cli wafv2 *-ip-set` commands there will be potential ban throughput and container counts for which the locking contention degrades performance.

## Usage

```sh
# run standalone
shopt -s extglob; go run *-standalone.go !(*-standalone|*-service).go [-l loglevel] [-p port]

# run as a service, see also the Dockerfile FIXME
shopt -s extglob; go run *-service.go !(*-standalone|*-service).go [-l loglevel] [-p port]
```

## Client interface

| Method | Endpoint           | Notes                                               |
| ------ | ------------------ | --------------------------------------------------- |
| GET    | /infraction/<ip>   | submit infraction for an ip                         |
| GET    | /state/infractions | enabled if loglevel <= 1, display infraction state  |
| GET    | /state/requests    | enabled if loglevel <= 1, display requests counters |
