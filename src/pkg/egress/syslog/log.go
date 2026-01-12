package syslog

// LogType defines the log types used within Cloud Foundry
// Their order in the code is as documented in https://docs.cloudfoundry.org/devguide/deploy-apps/streaming-logs.html#format
type LogType string

const (
	API  LogType = "API"
	STG  LogType = "STG"
	RTR  LogType = "RTR"
	LGR  LogType = "LGR"
	APP  LogType = "APP"
	SSH  LogType = "SSH"
	CELL LogType = "CELL"
)

// validLogTypes contains LogType prefixes for efficient lookup
var validLogTypes = map[LogType]struct{}{
	API:  {},
	STG:  {},
	RTR:  {},
	LGR:  {},
	APP:  {},
	SSH:  {},
	CELL: {},
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
