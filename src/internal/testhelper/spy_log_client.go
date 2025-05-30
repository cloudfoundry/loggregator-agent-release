package testhelper

import (
	"code.cloudfoundry.org/go-loggregator/v10"
	v2 "code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"sync"
)

type spyLogClient struct {
	mu       sync.Mutex
	_message []string
	_appID   []string

	// We use maps to ensure that we can query the keys
	_sourceType     map[string]struct{}
	_sourceInstance map[string]struct{}
}

func NewSpyLogClient() *spyLogClient {
	return &spyLogClient{
		_sourceType:     make(map[string]struct{}),
		_sourceInstance: make(map[string]struct{}),
	}
}

func (s *spyLogClient) EmitLog(message string, opts ...loggregator.EmitLogOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	env := &v2.Envelope{
		Tags: make(map[string]string),
	}

	for _, o := range opts {
		o(env)
	}

	s._message = append(s._message, message)
	s._appID = append(s._appID, env.SourceId)
	s._sourceType[env.GetTags()["source_type"]] = struct{}{}
	s._sourceInstance[env.GetInstanceId()] = struct{}{}
}

func (s *spyLogClient) Message() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._message
}

func (s *spyLogClient) AppID() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._appID
}

func (s *spyLogClient) SourceType() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy map so the orig does not escape the mutex and induce a race.
	m := make(map[string]struct{})
	for k := range s._sourceType {
		m[k] = struct{}{}
	}

	return m
}

func (s *spyLogClient) SourceInstance() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy map so the orig does not escape the mutex and induce a race.
	m := make(map[string]struct{})
	for k := range s._sourceInstance {
		m[k] = struct{}{}
	}

	return m
}
