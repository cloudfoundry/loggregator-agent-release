package syslog

import "strings"

// SourceType defines the source types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type SourceType string

const (
	SOURCE_API  SourceType = "API"
	SOURCE_STG  SourceType = "STG"
	SOURCE_RTR  SourceType = "RTR"
	SOURCE_LGR  SourceType = "LGR"
	SOURCE_APP  SourceType = "APP"
	SOURCE_SSH  SourceType = "SSH"
	SOURCE_CELL SourceType = "CELL"
	// TODO PROXY missing. Anything else as well? Also I guess there will be new ones in the future?
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

// AllSourceTypes returns all valid source types
func AllSourceTypes() []SourceType {
	types := make([]SourceType, 0, len(validSourceTypes))
	for t := range validSourceTypes {
		types = append(types, t)
	}
	return types
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

// ParseSourceType parses a string into a SourceType value
func ParseSourceType(s string) (SourceType, bool) {
	lt := SourceType(strings.ToUpper(s))
	return lt, lt.IsValid()
}
