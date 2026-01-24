package adapter

// CodexAdapter implements the Adapter interface for Codex CLI
type CodexAdapter struct {
	BaseAdapter
}

func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{
		BaseAdapter: BaseAdapter{
			name:        "codex",
			displayName: "Codex CLI",
			command:     "codex",
		},
	}
}

func (a *CodexAdapter) Name() string {
	return a.name
}

func (a *CodexAdapter) DisplayName() string {
	return a.displayName
}
