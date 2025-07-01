// Copyright 2025 The Reginald Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config_test

import (
	"slices"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/reginald-project/reginald-sdk-go/api"
	"github.com/reginald-project/reginald/internal/config"
	"github.com/reginald-project/reginald/internal/fspath"
	"github.com/reginald-project/reginald/internal/plugin"
)

const unionValueTestTaskID = "example/foo-0"

//nolint:cyclop,gocognit,gocyclo,maintidx // tests may be complex
func TestApplyTasks_UnionValue(t *testing.T) {
	t.Parallel()

	manifests := []*api.Manifest{
		{
			Name:        "reginald-example",
			Version:     "0.1.0",
			Domain:      "example",
			Description: "example config",
			Help:        "",
			Executable:  "",
			Config:      nil,
			Commands:    nil,
			Tasks: []api.Task{
				{
					Type:        "foo",
					Description: "does foo",
					RawConfig:   nil,
					Config: []api.ConfigType{
						api.UnionValue{
							Alternatives: []api.ConfigType{
								api.ConfigValue{
									KeyVal: api.KeyVal{
										Value: api.Value{
											Val:  []string{},
											Type: api.StringListValue,
										},
										Key: "foos",
									},
									Description: "config for foo",
								},
								api.ConfigValue{
									KeyVal: api.KeyVal{
										Value: api.Value{
											Val:  []int{},
											Type: api.IntListValue,
										},
										Key: "bar",
									},
									Description: "config for bar",
								},
								api.MappedValue{
									Key:         "foos",
									KeyType:     api.StringValue,
									Description: "config for foo",
									Values: []api.ConfigValue{
										{
											KeyVal: api.KeyVal{
												Value: api.Value{
													Val:  "",
													Type: api.StringValue,
												},
												Key: "string",
											},
											Description: "",
										},
										{
											KeyVal: api.KeyVal{
												Value: api.Value{
													Val:  false,
													Type: "bool",
												},
												Key: "bool",
											},
											Description: "",
										},
										{
											KeyVal: api.KeyVal{
												Value: api.Value{
													Val:  0,
													Type: api.IntValue,
												},
												Key: "int",
											},
											Description: "",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	file1 := `[[tasks]]
type = "example/foo"
foos = []`
	file2 := `[[tasks]]
type = "example/foo"
foos = ["hello", "world"]`
	file3 := `[[tasks]]
type = "example/foo"
bar = [1, 2, 3]`
	file4 := `[[tasks]]
type = "example/foo"
foos = { custom_key = { int = 3 } }`
	file5 := `[[tasks]]
type = "example/foo"

[tasks.foos."custom_1"]
string = "hello"
bool = true
int = 123

[tasks.foos."custom_2"]
string = "world"
bool = true
int = 321`

	defer func() {
		r := recover()

		if r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	cfg := parseFile(t, file1)

	t.Logf("applying from file1 to %+v", cfg)

	opts := config.TaskApplyOptions{
		Store:    newStore(t, manifests, cfg.Directory),
		Defaults: cfg.Defaults,
		Dir:      cfg.Directory,
	}

	var err error

	cfg.Tasks, err = config.ApplyTasks(t.Context(), cfg.RawTasks, opts)
	if err != nil {
		t.Errorf("failed to apply task values: %v", err)
	}

	tasks := cfg.Tasks
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.ID != unionValueTestTaskID {
		t.Errorf("expected ID %q, got %q", unionValueTestTaskID, task.ID)
	}

	t.Logf("file1 yielded: %+v", tasks)

	kv1, ok := task.Config.Get("foos")
	if !ok {
		t.Errorf("missing %q in config", "foos")
	}

	cmp1, err := api.NewKeyVal("foos", []string{})
	if err != nil {
		t.Fatal(err)
	}

	if !kv1.Equal(cmp1) {
		t.Errorf("expected %+v, got %+v", cmp1, kv1)
	}

	got1, err := kv1.StringSlice()
	if err != nil {
		t.Errorf("expected %T, got %T and error: %v", got1, kv1.Val, err)
	}

	if !slices.Equal(got1, []string{}) {
		t.Errorf("expected %v, got %v", []string{}, got1)
	}

	// FILE 2 //////////////////////////////////////////////////////////////////

	cfg = parseFile(t, file2)

	t.Logf("applying from file2 to %+v", cfg)

	cfg.Tasks, err = config.ApplyTasks(t.Context(), cfg.RawTasks, opts)
	if err != nil {
		t.Errorf("failed to apply task values: %v", err)
	}

	tasks = cfg.Tasks
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	task = tasks[0]
	if task.ID != unionValueTestTaskID {
		t.Errorf("expected ID %q, got %q", unionValueTestTaskID, task.ID)
	}

	t.Logf("file2 yielded: %+v", tasks)

	kv2, ok := task.Config.Get("foos")
	if !ok {
		t.Errorf("missing %q in config", "foos")
	}

	want2 := []string{"hello", "world"}

	cmp2, err := api.NewKeyVal("foos", want2)
	if err != nil {
		t.Fatal(err)
	}

	if !kv2.Equal(cmp2) {
		t.Errorf("expected %+v, got %+v", cmp2, kv2)
	}

	got2, err := kv2.StringSlice()
	if err != nil {
		t.Errorf("expected %T, got %T and error: %v", got2, kv2.Val, err)
	}

	if !slices.Equal(got2, want2) {
		t.Errorf("expected %v, got %v", want2, got2)
	}

	// FILE 3 //////////////////////////////////////////////////////////////////

	cfg = parseFile(t, file3)

	t.Logf("applying from file3 to %+v", cfg)

	cfg.Tasks, err = config.ApplyTasks(t.Context(), cfg.RawTasks, opts)
	if err != nil {
		t.Errorf("failed to apply task values: %v", err)
	}

	tasks = cfg.Tasks
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	task = tasks[0]
	if task.ID != unionValueTestTaskID {
		t.Errorf("expected ID %q, got %q", unionValueTestTaskID, task.ID)
	}

	t.Logf("file3 yielded: %+v", tasks)

	kv3, ok := task.Config.Get("bar")
	if !ok {
		t.Errorf("missing %q in config", "bar")
	}

	want3 := []int{1, 2, 3}

	cmp3, err := api.NewKeyVal("bar", want3)
	if err != nil {
		t.Fatal(err)
	}

	if !kv3.Equal(cmp3) {
		t.Errorf("expected %+v, got %+v", cmp3, kv3)
	}

	got3, err := kv3.IntSlice()
	if err != nil {
		t.Errorf("expected %T, got %T and error: %v", got3, kv3.Val, err)
	}

	if !slices.Equal(got3, want3) {
		t.Errorf("expected %v, got %v", want3, got3)
	}

	// FILE 4 //////////////////////////////////////////////////////////////////

	cfg = parseFile(t, file4)

	t.Logf("applying from file4 to %+v", cfg)

	cfg.Tasks, err = config.ApplyTasks(t.Context(), cfg.RawTasks, opts)
	if err != nil {
		t.Errorf("failed to apply task values: %v", err)
	}

	tasks = cfg.Tasks
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	task = tasks[0]
	if task.ID != unionValueTestTaskID {
		t.Errorf("expected ID %q, got %q", unionValueTestTaskID, task.ID)
	}

	t.Logf("file4 yielded: %+v", tasks)

	kv4, ok := task.Config.Get("foos")
	if !ok {
		t.Errorf("missing %q in config", "foos")
	}

	cmp4 := api.KeyVal{
		Value: api.Value{
			Val: api.KeyValues{
				{
					Value: api.Value{
						Val: api.KeyValues{
							{
								Value: api.Value{Val: "", Type: api.StringValue},
								Key:   "string",
							},
							{
								Value: api.Value{Val: false, Type: api.BoolValue},
								Key:   "bool",
							},
							{
								Value: api.Value{Val: 3, Type: api.IntValue},
								Key:   "int",
							},
						},
						Type: api.ConfigSliceValue,
					},
					Key: "custom_key",
				},
			},
			Type: api.ConfigSliceValue,
		},
		Key: "foos",
	}

	if !kv4.Equal(cmp4) {
		t.Errorf("expected %+v, got %+v", cmp4, kv4)
	}

	// FILE 5 //////////////////////////////////////////////////////////////////

	cfg = parseFile(t, file5)

	t.Logf("applying from file5 to %+v", cfg)

	cfg.Tasks, err = config.ApplyTasks(t.Context(), cfg.RawTasks, opts)
	if err != nil {
		t.Errorf("failed to apply task values: %v", err)
	}

	tasks = cfg.Tasks
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}

	task = tasks[0]
	if task.ID != unionValueTestTaskID {
		t.Errorf("expected ID %q, got %q", unionValueTestTaskID, task.ID)
	}

	t.Logf("file5 yielded: %+v", tasks)

	kv5, ok := task.Config.Get("foos")
	if !ok {
		t.Errorf("missing %q in config", "foos")
	}

	cmp5 := api.KeyVal{
		Value: api.Value{
			Val: api.KeyValues{
				{
					Value: api.Value{
						Val: api.KeyValues{
							{
								Value: api.Value{Val: "world", Type: api.StringValue},
								Key:   "string",
							},
							{
								Value: api.Value{Val: true, Type: api.BoolValue},
								Key:   "bool",
							},
							{
								Value: api.Value{Val: 321, Type: api.IntValue},
								Key:   "int",
							},
						},
						Type: api.ConfigSliceValue,
					},
					Key: "custom_2",
				},
				{
					Value: api.Value{
						Val: api.KeyValues{
							{
								Value: api.Value{Val: "hello", Type: api.StringValue},
								Key:   "string",
							},
							{
								Value: api.Value{Val: true, Type: api.BoolValue},
								Key:   "bool",
							},
							{
								Value: api.Value{Val: 123, Type: api.IntValue},
								Key:   "int",
							},
						},
						Type: api.ConfigSliceValue,
					},
					Key: "custom_1",
				},
			},
			Type: api.ConfigSliceValue,
		},
		Key: "foos",
	}

	if !kv5.Equal(cmp5) {
		t.Errorf("expected %+v, got %+v", cmp5, kv5)
	}
}

func parseFile(t *testing.T, file string) *config.Config {
	t.Helper()

	cfg := config.DefaultConfig()
	data := []byte(file)
	rawCfg := make(map[string]any)

	if err := toml.Unmarshal(data, &rawCfg); err != nil {
		t.Fatalf("Failed to parse config data: %v", err)
	}

	config.NormalizeKeys(rawCfg)

	decoderConfig := &mapstructure.DecoderConfig{ //nolint:exhaustruct // use default values
		DecodeHook: mapstructure.TextUnmarshallerHookFunc(),
		Result:     cfg,
	}

	d, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		t.Fatalf("failed to create mapstructure decoder: %v", err)
	}

	if err := d.Decode(rawCfg); err != nil {
		t.Fatalf("failed to decode config: %v", err)
	}

	return cfg
}

func newStore(t *testing.T, manifests []*api.Manifest, dir fspath.Path) *plugin.Store {
	t.Helper()

	store, err := plugin.NewStore(t.Context(), manifests, dir, nil)
	if err != nil {
		t.Fatalf("failed to create plugin Store: %v", err)
	}

	return store
}
