package bindings_test

import (
	"errors"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Drain Param Config", func() {
	It("sets OmitMetadata to false if the drain doesn't contain 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
	})

	It("sets OmitMetadata to true if the drain contains 'disable-metadata=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
		Expect(configedBindings[1].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to true if global flag is off", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, false)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeTrue())
	})

	It("sets OmitMetadata to false if global flag is off, but drain enables it", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?disable-metadata=false"}},
			{Drain: syslog.Drain{Url: "https://test.org/drain?omit-metadata=false"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, false)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].OmitMetadata).To(BeFalse())
		Expect(configedBindings[1].OmitMetadata).To(BeFalse())
	})

	It("sets internal tls to true if the drain contains 'ssl-strict-internal=true'", func() {
		bs := []syslog.Binding{
			{Drain: syslog.Drain{Url: "https://test.org/drain?ssl-strict-internal=true"}},
		}
		f := newStubFetcher(bs, nil)
		wf := bindings.NewDrainParamParser(f, true)

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
		wf := bindings.NewDrainParamParser(f, true)

		configedBindings, _ := wf.FetchBindings()
		Expect(configedBindings[0].DrainData).To(Equal(syslog.LOGS))
		Expect(configedBindings[1].DrainData).To(Equal(syslog.LOGS))
		Expect(configedBindings[2].DrainData).To(Equal(syslog.METRICS))
		Expect(configedBindings[3].DrainData).To(Equal(syslog.TRACES))
		Expect(configedBindings[4].DrainData).To(Equal(syslog.ALL))
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
		wf := bindings.NewDrainParamParser(f, true)

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
		wf := bindings.NewDrainParamParser(f, true)

		configedBindings, err := wf.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(configedBindings).To(HaveLen(2))
		Expect(configedBindings[0].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?disable-metadata=true"}))
		Expect(configedBindings[1].Drain).To(Equal(syslog.Drain{Url: "https://test.org/drain?omit-metadata=true"}))
	})

	It("Returns a error when fetching fails", func() {
		f := newStubFetcher(nil, errors.New("Ahhh an error"))
		wf := bindings.NewDrainParamParser(f, true)

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
