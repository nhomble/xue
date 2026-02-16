package main

const (
	defaultOutputDir     = ".xue"
	defaultSummaryPrompt = `Summarize this AI agent session in under 300 words.
Focus on: decisions made, questions raised, current state of the work, and any unresolved items.
Do not include greetings, pleasantries, or meta-commentary about the conversation itself.`
)

type Config struct {
	OutputDir string
	Verbose   bool
	NoSummary bool
	DryRun    bool
}

type PipelineConfig struct {
	Name       string             `yaml:"name"`
	Summarizer SummarizerConfig   `yaml:"summarizer"`
	Steps      []PipelineStep     `yaml:"steps"`
}

type SummarizerConfig struct {
	Model  string `yaml:"model"`
	Prompt string `yaml:"prompt"`
}

type PipelineStep struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Inject  string `yaml:"inject"`
}
