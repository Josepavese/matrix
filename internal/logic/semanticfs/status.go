package semanticfs

import (
	"fmt"
	"io/fs"
	"time"

	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/workspace"
)

type runStatus struct {
	ID                string    `json:"id"`
	Agent             string    `json:"agent_id"`
	Status            string    `json:"status"`
	Protocol          string    `json:"protocol"`
	RequestedModel    string    `json:"requested_model,omitempty"`
	ModelVerification string    `json:"model_verification,omitempty"`
	LogicalSession    string    `json:"logical_session_id,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type workspaceStatus struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RootPath     string    `json:"root_path,omitempty"`
	DefaultAgent string    `json:"default_agent_id,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *FS) status(kind, id string) (any, error) {
	if kind == "agents" {
		return s.agentStatus(id)
	}
	if s.source.Storage == nil {
		return nil, fmt.Errorf("semantic storage unavailable")
	}
	switch kind {
	case "runs":
		run, found, err := runtrace.NewStore(s.source.Storage).LoadRun(id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fs.ErrNotExist
		}
		return runStatus{ID: run.ID, Agent: run.AgentID, Status: run.Status, Protocol: run.Protocol, RequestedModel: run.RequestedModel, ModelVerification: run.ModelVerification, LogicalSession: run.LogicalSessionID, StartedAt: run.StartedAt, UpdatedAt: run.UpdatedAt}, nil
	case "workspaces":
		meta, found, err := workspace.LoadMeta(s.source.Storage, id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fs.ErrNotExist
		}
		return workspaceStatus{ID: meta.ID, Name: meta.Name, RootPath: meta.RootPath, DefaultAgent: meta.DefaultAgentID, UpdatedAt: meta.UpdatedAt}, nil
	default:
		return nil, fs.ErrNotExist
	}
}

func (s *FS) agentStatus(id string) (AgentView, error) {
	if s.source.Agents != nil {
		agents, err := s.source.Agents()
		if err != nil {
			return AgentView{}, err
		}
		for _, agent := range agents {
			if agent.ID == id {
				return agent, nil
			}
		}
		return AgentView{}, fs.ErrNotExist
	}
	if s.source.Storage == nil {
		return AgentView{}, fmt.Errorf("semantic storage unavailable")
	}
	return s.agentFromStorage(id)
}

func (s *FS) entityFile(kind, id, leaf string) (fs.File, error) {
	if leaf == "status.json" {
		status, err := s.status(kind, id)
		if err != nil {
			return nil, err
		}
		return jsonFile(leaf, status)
	}
	if _, err := s.status(kind, id); err != nil {
		return nil, err
	}
	if kind == "workspaces" && leaf == "capacity.json" {
		meta, _, err := workspace.LoadMeta(s.source.Storage, id)
		if err != nil {
			return nil, err
		}
		return jsonFile(leaf, workspace.ObserveCapacity(meta.RootPath, s.source.Capacity))
	}
	if kind == "runs" && leaf == "summary.txt" && s.source.IncludeSummaries {
		return s.summary(id)
	}
	return nil, fs.ErrNotExist
}

func (s *FS) summary(id string) (fs.File, error) {
	run, found, err := runtrace.NewStore(s.source.Storage).LoadRun(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fs.ErrNotExist
	}
	if run.Status != runtrace.StatusCompleted && run.Status != runtrace.StatusFailed && run.Status != runtrace.StatusCancelled {
		return nil, fmt.Errorf("semantic summary is terminal-only")
	}
	if len(run.Output) > 64<<10 {
		return nil, fmt.Errorf("semantic summary limit exceeded; use the bounded native summary getter")
	}
	return newFile("summary.txt", []byte(run.Output)), nil
}
