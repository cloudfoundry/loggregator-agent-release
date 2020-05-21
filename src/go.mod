module code.cloudfoundry.org/loggregator-agent-release/src

go 1.12

require (
	code.cloudfoundry.org/go-batching v0.0.0-20171020220229-924d2a9b48ac
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee
	code.cloudfoundry.org/go-envstruct v1.5.0
	code.cloudfoundry.org/go-loggregator v0.0.0-20190809213911-969cb33bee6a // pinned
	code.cloudfoundry.org/go-metric-registry v0.0.0-20200413202920-40d97c8804ec
	code.cloudfoundry.org/rfc5424 v0.0.0-20180905210152-236a6d29298a
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3
	github.com/apoydence/eachers v0.0.0-20181020210610-23942921fe77
	github.com/cloudfoundry/dropsonde v1.0.0 // pinned
	github.com/cloudfoundry/sonde-go v0.0.0-20171206171820-b33733203bb4 // pinned
	github.com/gogo/protobuf v1.3.1 // pinned
	github.com/golang/protobuf v1.4.2 // pinned
	github.com/gorilla/mux v1.7.4
	github.com/klauspost/compress v1.10.6 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/onsi/ginkgo v1.12.2
	github.com/onsi/gomega v1.10.1
	github.com/prometheus/client_golang v1.6.0 // indirect
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.10.0
	github.com/valyala/fasthttp v1.12.0
	golang.org/x/net v0.0.0-20200520182314-0ba52f642ac2
	google.golang.org/genproto v0.0.0-20200521103424-e9a78aa275b7 // indirect
	google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 v2.3.0
)
