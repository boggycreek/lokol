// Copyright (c) 2026 Boggy Creek Software LLC
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tui_test

import (
	"context"
	"sync"

	"github.com/boggycreek/lokol/liblokol/agent"
	"github.com/boggycreek/lokol/liblokol/regulator"
)

// MockSession is a mock implementation of agent.SessionCore for testing
// presentation layers without running an inference engine or touching the host filesystem.
type MockSession struct {
	mu sync.Mutex

	StreamTurnFunc    func(ctx context.Context, tokenChan chan<- string) (string, error)
	ExecuteActionFunc func(ctx context.Context, act *agent.Action) (string, error)
	AbortFunc         func(ctx context.Context) error
	GetSlotStatusFunc func(ctx context.Context) (*agent.SlotStatus, error)
	ResetFunc         func()

	// Mode and Persona tracking
	CurrentMode  agent.Mode
	WorkDir      string
	AgentName    string
	OperatorName string

	// Recorded interactions for test assertions
	UserMessages      []string
	AssistantMessages []string
	ActionResults     []string
	ExecutedActions   []*agent.Action
	AbortedCount      int
}

var _ agent.SessionCore = (*MockSession)(nil)

func NewMockSession() *MockSession {
	return &MockSession{}
}

func (m *MockSession) StreamTurn(ctx context.Context, tokenChan chan<- string) (string, error) {
	m.mu.Lock()
	fn := m.StreamTurnFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, tokenChan)
	}
	return "", nil
}

func (m *MockSession) ExecuteAction(ctx context.Context, act *agent.Action) (string, error) {
	m.mu.Lock()
	m.ExecutedActions = append(m.ExecutedActions, act)
	fn := m.ExecuteActionFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, act)
	}
	return "mock execution success", nil
}

func (m *MockSession) AppendUserMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UserMessages = append(m.UserMessages, content)
}

func (m *MockSession) AppendAssistantMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AssistantMessages = append(m.AssistantMessages, content)
}

func (m *MockSession) AppendActionResult(output string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.ActionResults = append(m.ActionResults, "[Error: "+err.Error()+"]\n"+output)
	} else {
		m.ActionResults = append(m.ActionResults, output)
	}
}

func (m *MockSession) Abort(ctx context.Context) error {
	m.mu.Lock()
	m.AbortedCount++
	fn := m.AbortFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx)
	}
	return nil
}

func (m *MockSession) GetSlotStatus(ctx context.Context) (*agent.SlotStatus, error) {
	m.mu.Lock()
	fn := m.GetSlotStatusFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx)
	}
	return &agent.SlotStatus{NCtx: 2048, NPromptTokens: 100}, nil
}

func (m *MockSession) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UserMessages = nil
	m.AssistantMessages = nil
	m.ActionResults = nil
	m.ExecutedActions = nil
	if m.ResetFunc != nil {
		m.ResetFunc()
	}
}

func (m *MockSession) GetMode() agent.Mode {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CurrentMode == "" {
		return agent.ModeGeneral
	}
	return m.CurrentMode
}

func (m *MockSession) SetMode(mode agent.Mode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentMode = mode
}

func (m *MockSession) GetWorkDir() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.WorkDir == "" {
		return "."
	}
	return m.WorkDir
}

func (m *MockSession) GetPersona() (string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agentName := m.AgentName
	if agentName == "" {
		agentName = "lokol"
	}
	operatorName := m.OperatorName
	if operatorName == "" {
		operatorName = "User"
	}
	return agentName, operatorName
}

func (m *MockSession) SetPersona(agentName, operatorName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if agentName != "" {
		m.AgentName = agentName
	}
	if operatorName != "" {
		m.OperatorName = operatorName
	}
}

func (m *MockSession) PruneToolOutputs(preserveRecent int) int {
	return 0
}

func (m *MockSession) CompactHistory(summaryLedger string, preserveRecent int) {
}

func (m *MockSession) SetLedgerProvider(provider func() string) {
}

func (m *MockSession) Compact(ctx context.Context, metrics *regulator.SlotMetrics) error {
	return nil
}

func (m *MockSession) HistoryLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.UserMessages) + len(m.AssistantMessages)
}

func (m *MockSession) GetHistory() []agent.Message {
	return nil
}

