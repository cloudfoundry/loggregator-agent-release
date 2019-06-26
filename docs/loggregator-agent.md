### Loggregator Agent
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

#### Compatability

The Loggregator Agent is compatable with Loggregator v96+.

#### Using Loggregator Agent

##### go-loggregator

There is Go client library: [go-loggregator][go-loggregator]. The client
library has several useful patterns along with examples to interact with a
Loggregator Agent.