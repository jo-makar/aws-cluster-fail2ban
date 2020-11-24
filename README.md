# aws-cluster-fail2ban
fail2ban for load-balanced services backed by clusters (eg AWS ECS or EKS) using AWS WAF and ip sets

This project itself can be implement as a service (ie as several containers) for services that handle massive amounts of connections and therefore high rates of potential bans.  The service-based implementation uses Redis to share state amongst the containers, the standalone version simply maintains state in memory.

Be aware that due to the optimistic locking provided by the `aws-cli wafv2 *-ip-set` commands there will be a potential ban throughput and container count for which the locking contention degrades performance.
