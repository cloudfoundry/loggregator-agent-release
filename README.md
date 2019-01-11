Loggregator Agent Release
[![slack.cloudfoundry.org][slack-badge]][loggregator-slack]
[![CI Badge][ci-badge]][ci-pipeline]
===================================================

Loggregator Agent is a Cloud Foundry component that forwards logs and metrics
into the Loggregator subsystem by taking traffic from the various emitter
sources (diego, router, etc) and routing that traffic to one or more dopplers.
An instance of Loggregator Agent runs on each VM in an environment and is
therefore co-located on the emitter sources.

Traffic is routed to Dopplers in the same AZ, but it can fall back to any
Doppler if none are available in the current AZ. All Loggregator Agent traffic
is randomly distributed across available Dopplers.

Loggregator Agent only listens to local network interfaces and all logs and
metrics are immediately signed before forwarding to Dopplers. This prevents
man-in-the-middle attacks and ensures data integrity.

* [Compatability](#compatability)
* [Example Deploy](#example-bosh-lite-deploy)
* [Usage](#using-loggregator-agent)
* [More Resources and Documentation](#more-resources-and-documentation)

## Compatability

The Loggregator Agent is compatable with Loggregator v96+.

## Example bosh lite deploy

```
bosh -e lite deploy -d loggregator-agent manifests/loggregator-agent.yml \
    --vars-file /tmp/loggregator-vars.yml \
    --vars-store=/tmp/agent-vars.yml
```

## Using Loggregator Agent

### go-loggregator

There is Go client library: [go-loggregator][go-loggregator]. The client
library has several useful patterns along with examples to interact with a
Loggregator Agent.

### VM Metrics

When the Loggregator System Metrics Agent is deployed along with the Loggregator Agent, 
it will emit the following metrics about the VM where it is deployed:

| Metric                               | Linux | Windows |
|--------------------------------------|-------|---------|
| system_mem_kb                        |   ✔️   |    ✔️    |
| system_mem_percent                   |   ✔️   |    ✔️    |
| system_swap_kb                       |   ✔️   |    ✔️    |
| system_swap_percent                  |   ✔️   |    ✔️    |
| system_load_1m                       |   ✔️   |         |
| system_load_5m                       |   ✔️   |         |
| system_load_15m                      |   ✔️   |         |
| system_cpu_user                      |   ✔️   |    ✔️    |
| system_cpu_sys                       |   ✔️   |    ✔️    |
| system_cpu_idle                      |   ✔️   |    ✔️    |
| system_cpu_wait                      |   ✔️   |    ✔️    |
| system_disk_system_percent           |   ✔️   |    ✔️    |
| system_disk_system_inode_percent     |   ✔️   |    ✔️    |
| system_disk_system_read_bytes        |   ✔️   |    ✔️    |
| system_disk_system_write_bytes       |   ✔️   |    ✔️    |
| system_disk_system_read_time         |   ✔️   |    ✔️    |
| system_disk_system_write_time        |   ✔️   |    ✔️    |
| system_disk_system_io_time           |   ✔️   |    ✔️    |
| system_disk_ephemeral_percent        |   ✔️   |    ✔️    |
| system_disk_ephemeral_inode_percent  |   ✔️   |    ✔️    |
| system_disk_ephemeral_read_bytes     |   ✔️   |    ✔️    |
| system_disk_ephemeral_write_bytes    |   ✔️   |    ✔️    |
| system_disk_ephemeral_read_time      |   ✔️   |    ✔️    |
| system_disk_ephemeral_write_time     |   ✔️   |    ✔️    |
| system_disk_ephemeral_io_time        |   ✔️   |    ✔️    |
| system_disk_persistent_percent       |   ✔️   |    ✔️    |
| system_disk_persistent_inode_percent |   ✔️   |    ✔️    |
| system_disk_persistent_read_bytes    |   ✔️   |    ✔️    |
| system_disk_persistent_write_bytes   |   ✔️   |    ✔️    |
| system_disk_persistent_read_time     |   ✔️   |    ✔️    |
| system_disk_persistent_write_time    |   ✔️   |    ✔️    |
| system_disk_persistent_io_time       |   ✔️   |    ✔️    |
| system_healthy                       |   ✔️   |    ✔️    |
| system_network_ip_forwarding         |   ✔️   |         |
| system_network_udp_no_ports          |   ✔️   |         |
| system_network_udp_in_errors         |   ✔️   |         |
| system_network_udp_lite_in_errors    |   ✔️   |         |
| system_network_tcp_active_opens      |   ✔️   |         |
| system_network_tcp_curr_estab        |   ✔️   |         |
| system_network_tcp_retrans_segs      |   ✔️   |         |
| system_network_bytes_sent            |   ✔️   |    ✔️    |
| system_network_bytes_received        |   ✔️   |    ✔️    |
| system_network_packets_sent          |   ✔️   |    ✔️    |
| system_network_packets_received      |   ✔️   |    ✔️    |
| system_network_error_in              |   ✔️   |    ✔️    |
| system_network_error_out             |   ✔️   |    ✔️    |
| system_network_drop_in               |   ✔️   |    ✔️    |
| system_network_drop_out              |   ✔️   |    ✔️    |

Note: these metrics are also available via HTTP in Prometheus format.

## More Resources and Documentation

### Roadmap

We communicate our long term planning using a [Product Road
Map][loggregator-roadmap], and are always looking to gather feedback and input
from Loggregator operators. Get in touch or file an issue if you have feature
suggestions you'd like to see added.

### Pivotal Tracker

Items marked as "In Flight" on the Roadmap are tracked as new Features in
[Pivotal Tracker][loggregator-tracker].


[slack-badge]:         https://slack.cloudfoundry.org/badge.svg
[loggregator-slack]:   https://cloudfoundry.slack.com/archives/loggregator
[ci-badge]:            https://loggregator.ci.cf-app.com/api/v1/pipelines/loggregator/jobs/loggregator-tests/badge
[ci-pipeline]:         https://loggregator.ci.cf-app.com/teams/main/pipelines/loggregator
[loggregator-tracker]: https://www.pivotaltracker.com/n/projects/993188
[loggregator-roadmap]: https://github.com/cloudfoundry/loggregator-release/projects/1
[go-loggregator]:      https://code.cloudfoundry.org/go-loggregator
