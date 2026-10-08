package semanticfs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/logic/runtrace"
	"github.com/Josepavese/matrix/internal/logic/workspace"
)

type dirFile struct {
	info     info
	entries  []fs.DirEntry
	position int
}

type directoryEntry struct {
	info
	owner    *FS
	fullPath string
}

func (e directoryEntry) Info() (fs.FileInfo, error) {
	f, err := e.owner.Open(e.fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func (s *FS) directory(name string, names []string) *dirFile {
	entries := make([]fs.DirEntry, 0, len(names))
	sort.Strings(names)
	for _, child := range names {
		entries = append(entries, directoryEntry{info: info{name: child, dir: !strings.HasSuffix(child, ".json") && !strings.HasSuffix(child, ".txt"), modified: s.epoch}, owner: s, fullPath: path.Join(name, child)})
	}
	return &dirFile{info: info{name: path.Base(name), dir: true, modified: s.epoch}, entries: entries}
}
func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }
func (*dirFile) Read([]byte) (int, error)     { return 0, fmt.Errorf("cannot read a directory") }
func (*dirFile) Close() error                 { return nil }
func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries := d.entries[d.position:]
		d.position = len(d.entries)
		return entries, nil
	}
	if d.position == len(d.entries) {
		return nil, io.EOF
	}
	end := min(len(d.entries), d.position+n)
	entries := d.entries[d.position:end]
	d.position = end
	return entries, nil
}

func (s *FS) category(kind string) (fs.File, error) {
	ids, err := s.ids(kind)
	if err != nil {
		return nil, err
	}
	if len(ids) > s.source.MaxEntries {
		return nil, fmt.Errorf("semantic entry limit exceeded; use the bounded native run/workspace APIs")
	}
	var names []string
	for _, id := range ids {
		name, err := DirectoryName(id)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return s.directory(kind, names), nil
}

func (s *FS) ids(kind string) ([]string, error) {
	if kind == "agents" && s.source.Agents != nil {
		agents, err := s.source.Agents()
		ids := make([]string, 0, len(agents))
		for _, agent := range agents {
			ids = append(ids, agent.ID)
		}
		return ids, err
	}
	prefixes := map[string]string{"agents": agentcfg.MetaKeyPrefix, "runs": runtrace.RunKey(""), "workspaces": workspace.MetaKeyPrefix}
	prefix, ok := prefixes[kind]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if s.source.Storage == nil {
		return nil, fmt.Errorf("semantic storage unavailable")
	}
	keys, err := s.source.Storage.List(prefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, strings.TrimPrefix(key, prefix))
	}
	return ids, nil
}

func (s *FS) entityDirectory(kind, id, name string) (fs.File, error) {
	if _, err := s.status(kind, id); err != nil {
		return nil, err
	}
	names := []string{"status.json"}
	if kind == "workspaces" {
		names = append(names, "capacity.json")
	}
	if kind == "runs" && s.source.IncludeSummaries {
		names = append(names, "summary.txt")
	}
	return s.directory(kind+"/"+name, names), nil
}

func (s *FS) agentFromStorage(id string) (AgentView, error) {
	data, err := s.source.Storage.Get(agentcfg.MetaKeyPrefix + id)
	if err != nil {
		return AgentView{}, err
	}
	if len(data) == 0 {
		return AgentView{}, fs.ErrNotExist
	}
	var meta struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return AgentView{}, err
	}
	return AgentView{ID: id, Kind: meta.Kind, Source: "stored_agent_metadata_active_state_unobserved"}, nil
}
