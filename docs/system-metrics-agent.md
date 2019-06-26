### System Metrics Agent
A standalone agent to provide VM system metrics via a prometheus-scrapable endpoint.

#### VM Metrics

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
| system_cpu_core_user                 |   ✔️   |    ✔️    |
| system_cpu_core_sys                  |   ✔️   |    ✔️    |
| system_cpu_core_idle                 |   ✔️   |    ✔️    |
| system_cpu_core_wait                 |   ✔️   |    ✔️    |
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
