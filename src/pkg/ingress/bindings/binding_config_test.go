package bindings_test

import (
	"errors"
	"log"
	"strings"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Drain Param Config", func() {
	var (
		logger = log.New(GinkgoWriter, "", 0)
	)
	It("sets OmitMetadata to false if the drain doesn't contain 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
	})

	It("sets OmitMetadata to true if the drain contains 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
		Expect(configedBindings[1].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to true if global flag is off", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, false, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to false if global flag is off, but drain enables it", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=false"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=false"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, false, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
		Expect(configedBindings[1].OmitMetadata).To(BeFalse())
	})

	It("sets internal tls to true if the drain contains 'ssl-strict-internal=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?ssl-strict-internal=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].InternalTls).To(BeTrue())
	})

	It("sets drain data appropriately'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-data=logs"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-data=metrics"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-data=traces"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-data=all"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].DrainData).To(Equal(syslog.LOGS))
		Expect(configedBindings[1].DrainData).To(Equal(syslog.LOGS))
		Expect(configedBindings[2].DrainData).To(Equal(syslog.METRICS))
		Expect(configedBindings[3].DrainData).To(Equal(syslog.TRACES))
		Expect(configedBindings[4].DrainData).To(Equal(syslog.ALL))
	})

	It("sets drain filter appropriately'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?include-log-types=app"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?include-log-types=app,stg,cell"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?exclude-log-types=rtr,cell,stg"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?exclude-log-types=rtr"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].LogFilter).To(Equal(NewLogTypeSet())) // Empty map defaults to all types
		Expect(configedBindings[1].LogFilter).To(Equal(NewLogTypeSet(syslog.LOG_APP)))
		Expect(configedBindings[2].LogFilter).To(Equal(NewLogTypeSet(syslog.LOG_APP, syslog.LOG_STG, syslog.LOG_CELL)))
		Expect(configedBindings[3].LogFilter).To(Equal(NewLogTypeSet(syslog.LOG_API, syslog.LOG_LGR, syslog.LOG_APP, syslog.LOG_SSH)))
		Expect(configedBindings[4].LogFilter).To(Equal(NewLogTypeSet(syslog.LOG_API, syslog.LOG_STG, syslog.LOG_LGR, syslog.LOG_APP, syslog.LOG_SSH, syslog.LOG_CELL)))
	})

	It("returns an error when both include-log-types and exclude-log-types are specified", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?include-log-types=app&exclude-log-types=rtr"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, err := wf.FetchBindings()
		Expect(err).To(HaveOccurred())
		Expect(configedBindings).To(HaveLen(0))
	})

	It("logs a warning when an unknown log type is provided", func() {
		var logOutput strings.Builder
		testLogger := log.New(&logOutput, "", log.LstdFlags)
		parser := bindings.NewDrainParamParser(newStubFetcher(nil, nil), false, testLogger)

		result := parser.NewLogTypeSet("app,unknown,rtr", false)

		// Should only contain APP and RTR, not the unknown type
		Expect(result).To(Equal(NewLogTypeSet(syslog.LOG_APP, syslog.LOG_RTR)))

		// Should have logged a warning
		Expect(logOutput.String()).To(ContainSubstring("ignoring"))
	})

	It("handles unknown log types in exclude mode", func() {
		var logOutput strings.Builder
		testLogger := log.New(&logOutput, "", log.LstdFlags)
		parser := bindings.NewDrainParamParser(newStubFetcher(nil, nil), false, testLogger)

		result := parser.NewLogTypeSet("rtr,unknown", true)

		// Should exclude only RTR (unknown type is ignored)
		expectedSet := NewLogTypeSet(syslog.LOG_API, syslog.LOG_STG, syslog.LOG_LGR, syslog.LOG_APP, syslog.LOG_SSH, syslog.LOG_CELL)
		Expect(result).To(Equal(expectedSet))

		// Should have logged a warning
		Expect(logOutput.String()).To(ContainSubstring("ignoring"))
	})

	It("sets drain data for old parameter appropriately'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-type=metrics"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-type=logs"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?drain-type=all"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?include-metrics-deprecated=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].DrainData).To(Equal(syslog.METRICS))
		Expect(configedBindings[1].DrainData).To(Equal(syslog.LOGS_NO_EVENTS))
		Expect(configedBindings[2].DrainData).To(Equal(syslog.LOGS))
		Expect(configedBindings[3].DrainData).To(Equal(syslog.LOGS_AND_METRICS))
		Expect(configedBindings[4].DrainData).To(Equal(syslog.ALL))
	})

	It("omits bindings with bad Drain URLs", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "   https://leading-spaces-are-invalid"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true, logger)

		configedBindings, err := wf.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(configedBindings).To(HaveLen(2))
		Expect(configedBindings[0].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}))
		Expect(configedBindings[1].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}))
	})

	It("Returns a error when fetching fails", func() {
		f := newStubFetcher(nil, errors.New("Ahhh an error"))
		wf := bindings.NewDrainParamParser(f, true, logger)

		_, err := wf.FetchBindings()
		Expect(err).To(MatchError("Ahhh an error"))
	})
})

type stubFetcher struct {
	bindings []syslog.Binding
	err      error
}

func newStubFetcher(bs []syslog.Binding, err error) *stubFetcher {
	return &stubFetcher{
		bindings: bs,
		err:      err,
	}
}

func (f *stubFetcher) FetchBindings() ([]syslog.Binding, error) {
	return f.bindings, f.err
}

func (f *stubFetcher) DrainLimit() int {
	return -1
}

func NewLogTypeSet(logTypes ...syslog.LogType) *syslog.LogTypeSet {
	set := make(syslog.LogTypeSet, len(logTypes))
	for _, t := range logTypes {
		set[t] = struct{}{}
	}
	return &set
}
