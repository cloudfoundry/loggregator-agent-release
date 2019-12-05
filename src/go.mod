module code.cloudfoundry.org/loggregator-agent

go 1.12

require (
	code.cloudfoundry.org/go-batching v0.0.0-20171020220229-924d2a9b48ac
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee
	code.cloudfoundry.org/go-envstruct v1.5.0
	code.cloudfoundry.org/go-loggregator v0.0.0-20190809213911-969cb33bee6a // pinned
	code.cloudfoundry.org/go-metric-registry v0.0.0-20191004164645-33b67ef0f7d1
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a
	code.cloudfoundry.org/tlsconfig v0.0.0-20191126220907-6c65973656e3
	github.com/apoydence/eachers v0.0.0-20181020210610-23942921fe77
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/cloudfoundry/sonde-go v0.0.0-20171206171820-b33733203bb4
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/gorilla/mux v1.7.3
	github.com/klauspost/compress v1.9.4 // indirect
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.7.1
	github.com/prometheus/client_golang v1.2.1 // indirect
	github.com/prometheus/client_model v0.0.0-20191202183732-d1d2010b5bee
	github.com/prometheus/common v0.7.0
	github.com/prometheus/procfs v0.0.8 // indirect
	github.com/square/certstrap v1.2.0 // indirect
	github.com/valyala/fasthttp v1.6.0
	golang.org/x/lint v0.0.0-20190313153728-d0100b6bd8b3 // indirect
	golang.org/x/net v0.0.0-20191204025024-5ee1b9f4859a
	golang.org/x/sys v0.0.0-20191204072324-ce4227a45e2e // indirect
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/tools v0.0.0-20190524140312-2c0ae7006135 // indirect
	google.golang.org/genproto v0.0.0-20191205163323-51378566eb59 // indirect
	google.golang.org/grpc v1.25.1
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.2.7
)
