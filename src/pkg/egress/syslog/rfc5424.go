package syslog

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"

	"code.cloudfoundry.org/go-loggregator/v9/rfc5424"
)

var findSpaces, findInvalidCharacters, findTrailingDashes *regexp.Regexp

func init() {
	findSpaces = regexp.MustCompile(`\s+`)
	findInvalidCharacters = regexp.MustCompile("[^-a-zA-Z0-9]+")
	findTrailingDashes = regexp.MustCompile("-+$")
}

const RFC5424TimeOffsetNum = "2006-01-02T15:04:05.999999-07:00"

// gaugeStructuredDataID contains the registered enterprise ID for the Cloud
// Foundry Foundation.
// See: https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers
const (
	gaugeStructuredDataID   = "gauge@47450"
	timerStructuredDataID   = "timer@47450"
	counterStructuredDataID = "counter@47450"
	eventStructuredDataID   = "event@47450"
	tagsStructuredDataID    = "tags@47450"
)

type ConverterOption func(*Converter)

func WithoutSyslogMetadata() ConverterOption {
	return func(c *Converter) {
		c.omitTags = true
	}
}

type Converter struct {
	omitTags bool
}

func NewConverter(opts ...ConverterOption) *Converter {
	c := &Converter{}

	for _, o := range opts {
		o(c)
	}

	return c
}

func (c *Converter) ToRFC5424(env *loggregator_v2.Envelope, defaultHostname string) ([][]byte, error) {
	hostname := c.BuildHostname(env, defaultHostname)

	appID := env.GetSourceId()

	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Log:
		return c.toRFC5424LogMessage(env, hostname, appID)
	case *loggregator_v2.Envelope_Gauge:
		return c.toRFC5424GaugeMessage(env, hostname, appID)
	case *loggregator_v2.Envelope_Timer:
		return c.toRFC5424TimerMessage(env, hostname, appID)
	case *loggregator_v2.Envelope_Counter:
		return c.toRFC5424CounterMessage(env, hostname, appID)
	case *loggregator_v2.Envelope_Event:
		return c.toRFC5424EventMessage(env, hostname, appID)
	default:
		return nil, nil
	}
}

func (c *Converter) BuildHostname(env *loggregator_v2.Envelope, defaultHostname string) string {
	hostname := defaultHostname

	envTags := env.GetTags()
	orgName, orgOk := envTags["organization_name"]
	spaceName, spaceOk := envTags["space_name"]
	appName, appOk := envTags["app_name"]
	if orgOk || spaceOk || appOk {
		sanitizedOrgName := c.sanitize(orgName)
		sanitizedSpaceName := c.sanitize(spaceName)
		sanitizedAppName := c.sanitize(appName)
		hostname = fmt.Sprintf("%s.%s.%s", c.truncate(sanitizedOrgName, 63), c.truncate(sanitizedSpaceName, 63), c.truncate(sanitizedAppName, 63))
	}

	return hostname
}

func (c *Converter) truncate(s string, num int) string {
	if len(s) <= num {
		return s
	}
	return s[:num]
}

func (c *Converter) sanitize(originalName string) string {
	return findTrailingDashes.ReplaceAllString(findInvalidCharacters.ReplaceAllString(findSpaces.ReplaceAllString(originalName, "-"), ""), "")
}

func (c *Converter) toRFC5424CounterMessage(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	counter := env.GetCounter()
	sd := rfc5424.StructuredData{ID: counterStructuredDataID, Parameters: []rfc5424.SDParam{{Name: "name", Value: counter.GetName()}, {Name: "total", Value: strconv.FormatUint(counter.GetTotal(), 10)}, {Name: "delta", Value: strconv.FormatUint(counter.GetDelta(), 10)}}}
	messageBinary, err := c.toRFC5424MetricMessage(env, hostname, appID, sd)
	return [][]byte{messageBinary}, err
}

func (c *Converter) toRFC5424GaugeMessage(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	gauges := make([][]byte, 0, 5)

	for name, g := range env.GetGauge().GetMetrics() {
		sd := rfc5424.StructuredData{ID: gaugeStructuredDataID, Parameters: []rfc5424.SDParam{{Name: "name", Value: name}, {Name: "value", Value: strconv.FormatFloat(g.GetValue(), 'g', -1, 64)}, {Name: "unit", Value: g.GetUnit()}}}
		guage, err := c.toRFC5424MetricMessage(env, hostname, appID, sd)
		if err != nil {
			return gauges, err
		}
		gauges = append(gauges, guage)
	}

	return gauges, nil
}

func (c *Converter) toRFC5424TimerMessage(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	timer := env.GetTimer()
	sd := rfc5424.StructuredData{ID: timerStructuredDataID, Parameters: []rfc5424.SDParam{{Name: "name", Value: timer.GetName()}, {Name: "start", Value: strconv.FormatInt(timer.GetStart(), 10)}, {Name: "stop", Value: strconv.FormatInt(timer.GetStop(), 10)}}}
	messageBinary, err := c.toRFC5424MetricMessage(env, hostname, appID, sd)
	return [][]byte{messageBinary}, err
}

func (c *Converter) toRFC5424EventMessage(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	event := env.GetEvent()
	sd := rfc5424.StructuredData{ID: eventStructuredDataID, Parameters: []rfc5424.SDParam{{Name: "title", Value: event.GetTitle()}, {Name: "body", Value: event.GetBody()}}}

	messageBinary, err := c.toRFC5424MetricMessage(env, hostname, appID, sd)
	return [][]byte{messageBinary}, err
}

func (c *Converter) toRFC5424LogMessage(env *loggregator_v2.Envelope, hostname, appID string) ([][]byte, error) {
	priority := c.genPriority(env.GetLog().Type)
	ts := time.Unix(0, env.GetTimestamp()).UTC()
	hostname = c.nilify(hostname)
	appID = c.nilify(appID)
	pid := c.nilify(generateProcessID(
		env.Tags["source_type"],
		env.InstanceId,
	))
	msg := appendNewline(removeNulls(env.GetLog().Payload))

	structuredDatas := []rfc5424.StructuredData{}
	baseSD := c.buildTagsStructuredData(env.GetTags())
	if baseSD.ID != "" {
		structuredDatas = append(structuredDatas, baseSD)
	}
	message := rfc5424.Message{
		Priority:       rfc5424.Priority(priority),
		Timestamp:      ts,
		Hostname:       hostname,
		AppName:        appID,
		ProcessID:      pid,
		Message:        msg,
		StructuredData: structuredDatas,
	}
	messageBinary, err := message.MarshalBinary()

	return [][]byte{messageBinary}, err
}

func (c *Converter) buildTagsStructuredData(tags map[string]string) rfc5424.StructuredData {
	if c.omitTags {
		return rfc5424.StructuredData{}
	}

	var tagKeys []string
	var tagsData []rfc5424.SDParam

	for k := range tags {
		tagKeys = append(tagKeys, k)
	}

	if len(tagKeys) == 0 {
		return rfc5424.StructuredData{}
	}

	sort.Strings(tagKeys)

	for _, k := range tagKeys {
		tagsData = append(tagsData, rfc5424.SDParam{Name: k, Value: tags[k]})
	}

	return rfc5424.StructuredData{ID: tagsStructuredDataID, Parameters: tagsData}
}

func (c *Converter) toRFC5424MetricMessage(env *loggregator_v2.Envelope, hostname, appID string, structuredData rfc5424.StructuredData) ([]byte, error) {
	ts := time.Unix(0, env.GetTimestamp()).UTC()
	hostname = c.nilify(hostname)
	appID = c.nilify(appID)
	pid := "[" + env.InstanceId + "]"
	priority := 14
	structuredDatas := []rfc5424.StructuredData{structuredData}
	baseSD := c.buildTagsStructuredData(env.GetTags())
	if baseSD.ID != "" {
		structuredDatas = append(structuredDatas, baseSD)
	}
	message := rfc5424.Message{
		Priority:       rfc5424.Priority(priority),
		Timestamp:      ts,
		Hostname:       hostname,
		AppName:        appID,
		ProcessID:      pid,
		Message:        []byte(""),
		StructuredData: structuredDatas, //TODO: Fix this to get both structured datas
	}
	messageBinary, err := message.MarshalBinary()
	return append(messageBinary, []byte(" \n")...), err
}

func (c *Converter) genPriority(logType loggregator_v2.Log_Type) int {
	switch logType {
	case loggregator_v2.Log_OUT:
		return 14
	case loggregator_v2.Log_ERR:
		return 11
	default:
		return -1
	}
}

func (c *Converter) nilify(x string) string {
	if x == "" {
		return "-"
	}
	return x
}
