package syslog

import "strings"

// LogSourceType defines the source types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type LogSourceType string

const (
	LOG_SOURCE_API    LogSourceType = "API"
	LOG_SOURCE_STG    LogSourceType = "STG"
	LOG_SOURCE_RTR    LogSourceType = "RTR"
	LOG_SOURCE_LGR    LogSourceType = "LGR"
	LOG_SOURCE_APP    LogSourceType = "APP"
	LOG_SOURCE_SSH    LogSourceType = "SSH"
	LOG_SOURCE_CELL   LogSourceType = "CELL"
	LOG_SOURCE_PROXY  LogSourceType = "PROXY"
	LOG_SOURCE_HEALTH LogSourceType = "HEALTH"
	LOG_SOURCE_SYS    LogSourceType = "SYS"
	LOG_SOURCE_STATS  LogSourceType = "STATS"
)

// validSourceTypes contains SourceType prefixes for efficient lookup
var validSourceTypes = map[LogSourceType]struct{}{
	LOG_SOURCE_API:  {},
	LOG_SOURCE_STG:  {},
	LOG_SOURCE_RTR:  {},
	LOG_SOURCE_LGR:  {},
	LOG_SOURCE_APP:  {},
	LOG_SOURCE_SSH:  {},
	LOG_SOURCE_CELL: {},
}

// IsValid checks if the provided SourceType is valid
func (lt LogSourceType) IsValid() bool {
	_, ok := validSourceTypes[lt]
	return ok
}

// ParseSourceType parses a string into a SourceType value
func ParseSourceType(s string) (LogSourceType, bool) {
	lt := LogSourceType(strings.ToUpper(s))
	return lt, lt.IsValid()
}

// AllSourceTypes returns all valid source types
func AllSourceTypes() []LogSourceType {
	types := make([]LogSourceType, 0, len(validSourceTypes))
	for t := range validSourceTypes {
		types = append(types, t)
	}
	return types
}

// ExtractPrefix extracts the prefix from a source_type tag (e.g., "APP/PROC/WEB/0" -> "APP")
func ExtractPrefix(sourceTypeTag string) string {
	if idx := strings.IndexByte(sourceTypeTag, '/'); idx != -1 {
		return sourceTypeTag[:idx]
	}
	return sourceTypeTag
}

// LogSourceTypeSet is a set of SourceTypes for efficient membership checking
type LogSourceTypeSet map[LogSourceType]struct{}

// Add adds a SourceType to the set
func (s LogSourceTypeSet) Add(lt LogSourceType) {
	s[lt] = struct{}{}
}

// Contains checks if the set contains a SourceType
func (s LogSourceTypeSet) Contains(lt LogSourceType) bool {
	_, exists := s[lt]
	return exists
}

// LogFilterMode determines how the log filter should be applied
type LogFilterMode int

const (
	// LogFilterModeInclude only includes logs matching the specified types (strict)
	LogFilterModeInclude LogFilterMode = iota
	// LogFilterModeExclude excludes logs matching the specified types (permissive)
	LogFilterModeExclude
)

// LogFilter encapsulates source type filtering configuration
type LogFilter struct {
	Types LogSourceTypeSet
	Mode  LogFilterMode
}

// NewLogFilter creates a new LogFilter with the given types and mode
func NewLogFilter(types LogSourceTypeSet, mode LogFilterMode) *LogFilter {
	return &LogFilter{
		Types: types,
		Mode:  mode,
	}
}

// ShouldInclude determines if a log with the given sourceTypeTag should be forwarded
// Include mode omits missing/unknown source types, exclude mode forwards them
func (f *LogFilter) ShouldInclude(sourceTypeTag string) bool {
	if f == nil {
		return true
	}

	if sourceTypeTag == "" {
		return f.Mode == LogFilterModeExclude
	}

	prefix := ExtractPrefix(sourceTypeTag)
	sourceType := LogSourceType(prefix)
	if !sourceType.IsValid() {
		return f.Mode == LogFilterModeExclude
	}

	inSet := f.Types.Contains(sourceType)
	if f.Mode == LogFilterModeInclude {
		return inSet
	}
	return !inSet
}
