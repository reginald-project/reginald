package config

import (
	"log/slog"
	"os"
	"testing"

	"github.com/anttikivi/reginald/pkg/task"
	"golang.org/x/term"
)

//nolint:maintidx
func Test_from(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cfgFile *File
		want    *Config
		reverse bool
	}{
		"default": {
			cfgFile: defaultConfigFile(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelInfo,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingDisabled": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Enabled = false

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: false,
					Format:  "json",
					Level:   slog.LevelInfo,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingFormat": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Format = "text"

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "text",
					Level:   slog.LevelInfo,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingLevelDebug": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Level = slog.LevelDebug

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelDebug,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingLevelWarn": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Level = slog.LevelWarn

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelWarn,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingLevelError": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Level = slog.LevelError

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelError,
					Output:  defaultLogOutput,
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"loggingOutputStderr": {
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Logging.Level = slog.LevelError
				cf.Logging.Output = "stderr"

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelError,
					Output:  "stderr",
				},
				Quiet:   false,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: false,
		},
		"notDefault": {
			cfgFile: defaultConfigFile(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelError,
					Output:  "stderr",
				},
				Quiet:   true,
				Tasks:   []task.Config{},
				Verbose: false,
			},
			reverse: true,
		},
		"tasksEquals": { //nolint:dupl
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Tasks = []map[string]any{
					{
						"type":  "test",
						"name":  "testing",
						"test":  13,
						"test2": 4.25,
						"abc":   "hello world",
					},
					{
						"type": "test2",
						"name": "testing2",
						"test": "str",
						"bool": false,
					},
				}

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelInfo,
					Output:  defaultLogOutput,
				},
				Quiet: false,
				Tasks: []task.Config{
					{
						Type: "test",
						Name: "testing",
						Settings: map[string]any{
							"test":  13,
							"test2": 4.25,
							"abc":   "hello world",
						},
					},
					{
						Type: "test2",
						Name: "testing2",
						Settings: map[string]any{
							"test": "str",
							"bool": false,
						},
					},
				},
				Verbose: false,
			},
			reverse: false,
		},
		"tasksNotEquals": { //nolint:dupl
			cfgFile: (func() *File {
				cf := defaultConfigFile()
				cf.Tasks = []map[string]any{
					{
						"type":  "test",
						"name":  "testing",
						"test":  13,
						"test2": 4.25,
						"abc":   "hello world",
					},
					{
						"type": "test2",
						"name": "testing2",
						"test": "str",
						"bool": false,
					},
				}

				return cf
			})(),
			want: &Config{
				Color:      term.IsTerminal(int(os.Stdout.Fd())),
				ConfigFile: "",
				Directory:  "",
				Logging: LoggingConfig{
					Enabled: true,
					Format:  "json",
					Level:   slog.LevelInfo,
					Output:  defaultLogOutput,
				},
				Quiet: false,
				Tasks: []task.Config{
					{
						Type: "test",
						Name: "testing",
						Settings: map[string]any{
							"test":  13,
							"test2": 4.25,
							"abc":   "hello world",
						},
					},
					{
						Type: "test2",
						Name: "testing2",
						Settings: map[string]any{
							"test": "str",
							"bool": true,
						},
					},
				},
				Verbose: false,
			},
			reverse: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, err := tt.cfgFile.from()
			if err != nil {
				t.Errorf("File.from() failed: %v", err)
			}

			if tt.reverse {
				if c.Equal(tt.want) {
					t.Errorf("File.from(%v) = %v, want != %v", tt.cfgFile, c, tt.want)
				}
			} else {
				if !c.Equal(tt.want) {
					t.Errorf("File.from(%v) = %v, want %v", tt.cfgFile, c, tt.want)
				}
			}
		})
	}
}
