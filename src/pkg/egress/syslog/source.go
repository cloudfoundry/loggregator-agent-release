package syslog

import "strings"

// SourceType defines the source types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type SourceType string

const (
	SOURCE_API    SourceType = "API"
	SOURCE_STG    SourceType = "STG"
	SOURCE_RTR    SourceType = "RTR"
	SOURCE_LGR    SourceType = "LGR"
	SOURCE_APP    SourceType = "APP"
	SOURCE_SSH    SourceType = "SSH"
	SOURCE_CELL   SourceType = "CELL"
	SOURCE_PROXY  SourceType = "PROXY"
	SOURCE_HEALTH SourceType = "HEALTH"
	SOURCE_SYS    SourceType = "SYS"
	SOURCE_STATS  SourceType = "STATS"
)

// validSourceTypes contains SourceType prefixes for efficient lookup
var validSourceTypes = map[SourceType]struct{}{
	SOURCE_API:  {},
	SOURCE_STG:  {},
	SOURCE_RTR:  {},
	SOURCE_LGR:  {},
	SOURCE_APP:  {},
	SOURCE_SSH:  {},
	SOURCE_CELL: {},
}

// IsValid checks if the provided SourceType is valid
func (lt SourceType) IsValid() bool {
	_, ok := validSourceTypes[lt]
	return ok
}

// ParseSourceType parses a string into a SourceType value
func ParseSourceType(s string) (SourceType, bool) {
	lt := SourceType(strings.ToUpper(s))
	return lt, lt.IsValid()
}

// AllSourceTypes returns all valid source types
func AllSourceTypes() []SourceType {
	types := make([]SourceType, 0, len(validSourceTypes))
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

// SourceTypeSet is a set of SourceTypes for efficient membership checking
type SourceTypeSet map[SourceType]struct{}

// Add adds a SourceType to the set
func (s SourceTypeSet) Add(lt SourceType) {
	s[lt] = struct{}{}
}

// Contains checks if the set contains a SourceType
func (s SourceTypeSet) Contains(lt SourceType) bool {
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
	Types SourceTypeSet
	Mode  LogFilterMode
}

// NewLogFilter creates a new LogFilter with the given types and mode
func NewLogFilter(types SourceTypeSet, mode LogFilterMode) *LogFilter {
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
	sourceType := SourceType(prefix)
	if !sourceType.IsValid() {
		return f.Mode == LogFilterModeExclude
	}

	inSet := f.Types.Contains(sourceType)
	if f.Mode == LogFilterModeInclude {
		return inSet
	}
	return !inSet
}
