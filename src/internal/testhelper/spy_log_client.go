package testhelper

import (
	"sync"

	"code.cloudfoundry.org/go-loggregator/v10"
	v2 "code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
)

type SpyLogClient struct {
	mu       sync.Mutex
	_message []string
	_appID   []string

	// We use maps to ensure that we can query the keys
	_sourceType map[string]struct{}
}

func NewSpyLogClient() *SpyLogClient {
	return &SpyLogClient{
		_sourceType: make(map[string]struct{}),
	}
}

func (s *SpyLogClient) EmitLog(message string, opts ...loggregator.EmitLogOption) {
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
}

func (s *SpyLogClient) Message() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._message
}

func (s *SpyLogClient) AppID() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._appID
}

func (s *SpyLogClient) SourceType() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy map so the original does not escape the mutex and induce a race.
	m := make(map[string]struct{})
	for k := range s._sourceType {
		m[k] = struct{}{}
	}

	return m
}
