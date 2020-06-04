package syslog

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

var sanitizeRe1, sanitizeRe2, sanitizeRe3 *regexp.Regexp

func init() {
	sanitizeRe1 = regexp.MustCompile("\\s+")
	sanitizeRe2 = regexp.MustCompile("[^-a-zA-Z0-9]+")
	sanitizeRe3 = regexp.MustCompile("-+$")
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

func ToRFC5424(env *loggregator_v2.Envelope, defaultHostname string) ([][]byte, error) {
	if len(defaultHostname) > 255 {
		return nil, invalidValue("Hostname", defaultHostname)
	}

	hostname := buildHostname(env, defaultHostname)

	appID := env.GetSourceId()
	if len(appID) > 48 {
		return nil, invalidValue("AppName", appID)
	}

	if len(env.InstanceId) > 128 {
		return nil, invalidValue("AppName", appID)
	}

	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Log:
		return [][]byte{
			toRFC5424LogMessage(env, hostname, appID),
		}, nil
	case *loggregator_v2.Envelope_Gauge:
		return toRFC5424GaugeMessage(env, hostname, appID), nil
	case *loggregator_v2.Envelope_Timer:
		return [][]byte{
			toRFC5424TimerMessage(env, hostname, appID),
		}, nil
	case *loggregator_v2.Envelope_Counter:
		return [][]byte{
			toRFC5424CounterMessage(env, hostname, appID),
		}, nil
	case *loggregator_v2.Envelope_Event:
		return [][]byte{
			toRFC5424EventMessage(env, hostname, appID),
		}, nil
	default:
		return nil, nil
	}
}

func buildHostname(env *loggregator_v2.Envelope, defaultHostname string) string {
	hostname := defaultHostname

	envTags := env.GetTags()
	orgName, orgOk := envTags["organization_name"]
	spaceName, spaceOk := envTags["space_name"]
	appName, appOk := envTags["app_name"]
	if orgOk || spaceOk || appOk {
		hostname = fmt.Sprintf("%s.%s.%s", sanitize(orgName), sanitize(spaceName), sanitize(appName))
		if len(hostname) > 255 {
			hostname = hostname[:255]
		}
	}

	return hostname
}

func sanitize(originalName string) string {
	return sanitizeRe3.ReplaceAllString(sanitizeRe2.ReplaceAllString(sanitizeRe1.ReplaceAllString(originalName, "-"), ""), "")
}

func invalidValue(property, value string) error {
	return fmt.Errorf("Invalid value \"%s\" for property %s \n", value, property)
}

func toRFC5424CounterMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	counter := env.GetCounter()
	sd := `[` + counterStructuredDataID + ` name="` + counter.GetName() + `" total="` + strconv.FormatUint(counter.GetTotal(), 10) + `" delta="` + strconv.FormatUint(counter.GetDelta(), 10) + `"]`

	return toRFC5424MetricMessage(env, hostname, appID, sd)
}

func toRFC5424GaugeMessage(env *loggregator_v2.Envelope, hostname, appID string) [][]byte {
	gauges := make([][]byte, 0, 5)

	for name, g := range env.GetGauge().GetMetrics() {
		sd := `[` + gaugeStructuredDataID + ` name="` + name + `" value="` + strconv.FormatFloat(g.GetValue(), 'g', -1, 64) + `" unit="` + g.GetUnit() + `"]`
		gauges = append(gauges, toRFC5424MetricMessage(env, hostname, appID, sd))
	}

	return gauges
}

func toRFC5424TimerMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	timer := env.GetTimer()
	sd := fmt.Sprintf(`[%s name="%s" start="%d" stop="%d"]`, timerStructuredDataID, timer.GetName(), timer.GetStart(), timer.GetStop())

	return toRFC5424MetricMessage(env, hostname, appID, sd)
}

func toRFC5424EventMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	event := env.GetEvent()
	sd := fmt.Sprintf(`[%s title="%s" body="%s"]`, eventStructuredDataID, event.GetTitle(), event.GetBody())

	return toRFC5424MetricMessage(env, hostname, appID, sd)
}

func toRFC5424LogMessage(env *loggregator_v2.Envelope, hostname, appID string) []byte {
	priority := genPriority(env.GetLog().Type)
	ts := time.Unix(0, env.GetTimestamp()).UTC().Format(RFC5424TimeOffsetNum)
	hostname = nilify(hostname)
	appID = nilify(appID)
	pid := nilify(generateProcessID(
		env.Tags["source_type"],
		env.InstanceId,
	))
	msg := appendNewline(removeNulls(env.GetLog().Payload))

	structuredData := buildTagsStructuredData(env.GetTags())

	message := make([]byte, 0, 20+len(priority)+len(ts)+len(hostname)+len(appID)+len(pid)+len(msg))
	message = append(message, []byte("<"+priority+">1 ")...)
	message = append(message, []byte(ts+" ")...)
	message = append(message, []byte(hostname+" ")...)
	message = append(message, []byte(appID+" ")...)
	message = append(message, []byte(pid+" - ")...)
	message = append(message, []byte(structuredData+" ")...)
	message = append(message, msg...)

	return message
}

func buildTagsStructuredData(tags map[string]string) string {
	var tagKeys []string
	var tagsData []string

	for k := range tags {
		tagKeys = append(tagKeys, k)
	}

	if len(tagKeys) == 0 {
		return ""
	}

	sort.Strings(tagKeys)

	for _, k := range tagKeys {
		tagsData = append(tagsData, fmt.Sprintf(`%s="%s"`, k, tags[k]))
	}

	return fmt.Sprintf("[%s %s]", tagsStructuredDataID, strings.Join(tagsData, " "))
}

func toRFC5424MetricMessage(env *loggregator_v2.Envelope, hostname, appID, structuredData string) []byte {
	ts := time.Unix(0, env.GetTimestamp()).UTC().Format(RFC5424TimeOffsetNum)
	hostname = nilify(hostname)
	appID = nilify(appID)
	pid := "[" + env.InstanceId + "]"
	priority := "14"

	structuredData += buildTagsStructuredData(env.GetTags())

	message := make([]byte, 0, 20+len(priority)+len(ts)+len(hostname)+len(appID)+len(pid)+len(structuredData))
	message = append(message, []byte("<"+priority+">1 ")...)
	message = append(message, []byte(ts+" ")...)
	message = append(message, []byte(hostname+" ")...)
	message = append(message, []byte(appID+" ")...)
	message = append(message, []byte(pid+" - ")...)
	message = append(message, []byte(structuredData+" \n")...)

	return message
}

func genPriority(logType loggregator_v2.Log_Type) string {
	switch logType {
	case loggregator_v2.Log_OUT:
		return "14"
	case loggregator_v2.Log_ERR:
		return "11"
	default:
		return "-1"
	}
}

func nilify(x string) string {
	if x == "" {
		return "-"
	}
	return x
}
