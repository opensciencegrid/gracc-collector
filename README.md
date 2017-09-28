[![Build Status](https://travis-ci.org/opensciencegrid/gracc-collector.svg?branch=master)](https://travis-ci.org/opensciencegrid/gracc-collector)

# Overview

The gracc-collector is a "Gratia-Compatible Collector" that acts as a 
transitional interface between the obsolete [Gratia](https://sourceforge.net/projects/gratia/)
accounting collector and probes and the new GRÅCC accounting system.

It listens for bundles of records (as would be sent via replication from a 
Gratia collector or from a Gratia probe) on HTTP, processes the bundle into 
individual usage records, and sends those to RabbitMQ or another 
AMQP 0.9.1 broker.

# Configuration

The config file is [TOML](https://github.com/toml-lang/toml) format.
Config options can also be specified by environment variables, shown
below in parentheses. Environment variables override file settings.

    address = "localhost" # address to listen on (GRACC_ADDRESS)
    port = "8888"         # port to listen on (GRACC_PORT)
    timeout = "60s"       # HTTP connection timeout (GRACC_TIMEOUT)
    loglevel = "debug"    # log level [debug|info|warn|error|fatal|panic] (GRACC_LOGLEVEL)
    
    [AMQP]
    scheme = "amqp"       # AMQP URI scheme [amqp|amqps] (GRACC_AMQP_SCHEME)
    host = "localhost"    # AMQP broker (GRACC_AMQP_HOST)
    port = "5672"         # (GRACC_AMQP_PORT)
    vhost = ""            # (GRACC_AMQP_VHOST)
    exchange = ""         # (GRACC_AMQP_EXCHANGE)
    routingKey = ""       # (GRACC_AMQP_ROUTINGKEY)
    durable = true        # keep exchange between server restarts (GRACC_AMQP_DURABLE)
    autoDelete = true     # delete exchange when there are no remaining bindings (GRACC_AMQP_AUTODELETE)
    user = "guest"        # (GRACC_AMQP_USER)
    password = "guest"    # (GRACC_AMQP_PASSWORD)
    format = "raw"        # format to send record in [raw|xml|json] (GRACC_AMQP_FORMAT)
    retry = "10s"         # AMQP connection retry interval (GRACC_AMQP_RETRY)
    skipVerify = false    # skip TLS cert verification (GRACC_AMQP_SKIPVERIFY)

# Usage

    gracc-collector [-c <config file>] [-l <log file>] [-pprof on|<address:port>]

Where `<log file>` can be "stdout", "stderr", or a file name; default is "stderr".
`-pprof on` will expose [pprof](https://blog.golang.org/profiling-go-programs) on the main http server at `/debug/pprof/`. 
`-pprof <address:port>` will expose it on a separate http server as specified, also at `/debug/pprof/`.

See `sample/gracc.service` for a sample systemd unit configuration. Copy the file (with 
appropriate changes) to `/usr/lib/systemd/system/` then use standard systemd commands to
control the process.

* Start: `systemctl start gracc.service`
* Stop:  `systemctl stop gracc.service`
* Restart:  `systemctl restart gracc.service`
* Refresh log file:  `systemctl kill --signal=SIGUSR1 gracc.service`
* Toggle debug logging:  `systemctl kill --signal=SIGUSR2 gracc.service`

See `sample/gracc.logrotate` for a sample logrotate configuration. Copy the file (with
appropriate changes) to `/etc/logrotate.d/gracc`.

# Creating a Release

To create a release, one must do the following:

1.  Merge a commit to master updating the release notes in `README.md` and `gracc-collector.spec`.
2.  Create a release in the `gracc-collector` GitHub repository; this will create a new release.
3.  Head over to the OSG packaging repository and update the `gracc-collector` packaging there;
    you will need to at least copy over the `gracc-collector.spec` and the update the git SHA1
    associated with the release. Steps 1-3 should take 15 minutes.
4.  Utilize the OSG build tools to do an official build of the `gracc-collector` RPM in Koji.
    This should take 15 minutes.
5.  Wait until the GOC synchronizes the updated `gracc-collector` RPM to the `osg-development`
    repository.  Estimated wait time is 2 hours.
6.  Run `make release`; this will update the local Dockerfile and perform a commit.  Run
    `docker build .` to ensure the generated image contains the correct `gracc-collector` version.
7.  If all looks good, push the updated Dockerfile to GitHub; this will trigger the DockerHub
    build.

# Release Notes

### v1.1.7

* Allow skipping verification of RabbitMQ TLS certs.

### v1.1.6

* Allow Dockerfile releases to be made via osg-development repository.
* Add appropriate license file.

### v1.1.5

* Add support for X-Forwarded-For / X-Real-IP headers, for when the collector is
  running behind a HTTP proxy

### v1.1.4

* Fix issue causing collector to never see AMQP unblock signal.

### v1.1.3

* Better handling of AMQP connection blocking: If the connection is blocked 
  for a long time, existing HTTP connections will time out and return an error, 
  rather than staying open indefinitely. New HTTP connections will immediately 
  return an error.
* Improved error handling and HTTP responses.

### v1.1.2

Fix resource leak when rabbitmq is down.

### v1.1.1

Add configurable AMQP scheme to support TLS connections to broker.

### v1.1.0

Accept UsageRecord (treated identically as JobUsageRecord).

### v1.0.0

* Feature-complete for initial production deployment to replace Gratia collector.
* Accept XML record bundles ("multiupdate") as would typically be sent by a probe.

### v0.4.0

* Allow config options to be set by environment variable. 
  Precedence: env var > config file > default.
* Change timeout and AMQP.retry config options to duration strings.
  A duration string is a possibly signed sequence of decimal numbers, each with
  optional fraction and a unit suffix, such as "300ms", "-1.5h" or "2h45m". 
  Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". 

### v0.03.01

Fully flatten JobUsageRecord into simple key:value hash map.

### v0.03.00

Revised JSON output structure based on OGF UsageRecord and switch default output to JSON.

[Mapping documentation](https://opensciencegrid.github.io/gracc/dev-docs/raw-records/)

### v0.02.00

Significant rewrite to simplify while maintaining robust error handling.

* Removed unneeded file, elasticsearch, and kafka outputs.
* One AMQP channel is opened for each bundle received; the HTTP connection
  from the probe or collector is not closed until all records are confirmed
  by the AMQP broker. If an error occurs or a record is returned then a 500
  error code is returned.

### v0.01.10

* Handle AMQP returned records
* Improved parsing of record bundles

### v0.01.09

* Better handling for AMQP communication errors

### v0.01.08

* Support multiple AMQP workers
* Timeout if output filter is blocking too long, respond to request with error code so bundle gets re-sent later
* Improved log messages and responses

### v0.01.07

Created internal queues for outputting records.

### v0.01.05

Add basic collector statistics served at /stats.

### v0.01.04

Add option to write records to RabbitMQ.

### v0.01.03

Add option to write records to file.

### v0.01.02

Add option to send records to Kafka topic.

### v0.01.01

Initial dev version, send records to Elasticsearch.
