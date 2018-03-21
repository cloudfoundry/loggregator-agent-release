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
[ci-badge]:            https://loggregator.ci.cf-app.com/api/v1/pipelines/products/jobs/loggregator-agent-tests/badge
[ci-pipeline]:         https://loggregator.ci.cf-app.com/
[loggregator-tracker]: https://www.pivotaltracker.com/n/projects/993188
[loggregator-roadmap]: https://github.com/cloudfoundry/loggregator-release/projects/1
[go-loggregator]:      https://code.cloudfoundry.org/go-loggregator
