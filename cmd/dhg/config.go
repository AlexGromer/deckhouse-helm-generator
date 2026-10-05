package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

// defaultConfigFile is loaded from the working directory when present.
const defaultConfigFile = ".dhg.yaml"

// applyConfigFile sets flags of cmd from a YAML file whose keys are flag
// names, e.g.
//
//	chart-name: myapp
//	mode: separate
//	with: [flux, policies]
//	feature-opt: ["flux.namespace=prod"]
//	plugin: ["example.com/v1/Widget=./bin/widget"]
//
// Flags given on the command line win over the file. Unknown keys are errors,
// so typos do not pass silently. With path == "", .dhg.yaml in the working
// directory is used if it exists.
func applyConfigFile(cmd *cobra.Command, path string) error {
	if path == "" {
		if _, err := os.Stat(defaultConfigFile); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		path = defaultConfigFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}

	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		flag := cmd.Flags().Lookup(key)
		if flag == nil || key == "config" {
			return fmt.Errorf("config %s: unknown key %q (keys are `dhg %s` flag names)", path, key, cmd.Name())
		}
		if flag.Changed {
			continue
		}
		if err := setFlagFromConfig(flag, cfg[key]); err != nil {
			return fmt.Errorf("config %s: %s: %w", path, key, err)
		}
	}
	return nil
}

func setFlagFromConfig(flag *pflag.Flag, value interface{}) error {
	list, isList := value.([]interface{})
	if !isList {
		return flag.Value.Set(fmt.Sprint(value))
	}
	if _, ok := flag.Value.(pflag.SliceValue); !ok {
		return fmt.Errorf("expected a single value, got a list")
	}
	items := make([]string, len(list))
	for i, item := range list {
		items[i] = fmt.Sprint(item)
	}
	return flag.Value.(pflag.SliceValue).Replace(items)
}
