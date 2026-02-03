package syslog

import "strings"

// LogType defines the log types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type LogType string

const (
	LOG_API  LogType = "API"
	LOG_STG  LogType = "STG"
	LOG_RTR  LogType = "RTR"
	LOG_LGR  LogType = "LGR"
	LOG_APP  LogType = "APP"
	LOG_SSH  LogType = "SSH"
	LOG_CELL LogType = "CELL"
)

// validLogTypes contains LogType prefixes for efficient lookup
var validLogTypes = map[LogType]struct{}{
	LOG_API:  {},
	LOG_STG:  {},
	LOG_RTR:  {},
	LOG_LGR:  {},
	LOG_APP:  {},
	LOG_SSH:  {},
	LOG_CELL: {},
}

// IsValid checks if the provided LogType is valid
func (lt LogType) IsValid() bool {
	_, ok := validLogTypes[lt]
	return ok
}

// AllLogTypes returns all valid log types
func AllLogTypes() []LogType {
	types := make([]LogType, 0, len(validLogTypes))
	for t := range validLogTypes {
		types = append(types, t)
	}
	return types
}

// LogTypeSet is a set of LogTypes for efficient membership checking
type LogTypeSet map[LogType]struct{}

// Add adds a LogType to the set
func (s LogTypeSet) Add(lt LogType) {
	s[lt] = struct{}{}
}

// Contains checks if the set contains a LogType
func (s LogTypeSet) Contains(lt LogType) bool {
	_, exists := s[lt]
	return exists
}

// ParseLogType parses a string into a LogType value
func ParseLogType(s string) (LogType, bool) {
	lt := LogType(strings.ToUpper(s))
	return lt, lt.IsValid()
}
