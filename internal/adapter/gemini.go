package adapter

// GeminiAdapter implements the Adapter interface for Gemini CLI
type GeminiAdapter struct {
	BaseAdapter
}

func NewGeminiAdapter() *GeminiAdapter {
	return &GeminiAdapter{
		BaseAdapter: BaseAdapter{
			name:        "gemini",
			displayName: "Gemini CLI",
			command:     "gemini",
		},
	}
}

func (a *GeminiAdapter) Name() string {
	return a.name
}

func (a *GeminiAdapter) DisplayName() string {
	return a.displayName
}
