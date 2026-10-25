package regen

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"go.yaml.in/yaml/v3"
)

func bumpSemver(version, policy string) (string, error) {
	if policy == "none" || policy == "" {
		return version, nil
	}

	parts := strings.Split(version, "-")
	base := parts[0]
	nums := strings.Split(base, ".")

	if len(nums) != 3 {
		return "", fmt.Errorf("invalid semver: %s", version)
	}

	major, err := strconv.Atoi(nums[0])
	if err != nil {
		return "", err
	}
	minor, err := strconv.Atoi(nums[1])
	if err != nil {
		return "", err
	}
	patch, err := strconv.Atoi(nums[2])
	if err != nil {
		return "", err
	}

	switch policy {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	default:
		return "", fmt.Errorf("unknown bump policy: %s", policy)
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}

func (p *Plan) Bump(policy string, theirs *generator.OutputSet) error {
	if policy == "none" || policy == "" {
		return nil
	}

	// Find changed charts by looking at FilePlan
	changedDirs := make(map[string]bool)
	for _, fp := range p.Files {
		if fp.Action != ActionUnchanged && fp.Action != ActionKept {
			dir := path.Dir(filepath.ToSlash(fp.Path))
			changedDirs[dir] = true

			// Also mark all parent charts as changed
			parts := strings.Split(dir, "/")
			for i := len(parts); i > 0; i-- {
				parent := path.Join(parts[:i]...)
				changedDirs[parent] = true
			}
			changedDirs["."] = true
		}
	}

	for i, fp := range p.Files {
		if strings.HasSuffix(fp.Path, "Chart.yaml") {
			chartDir := path.Dir(filepath.ToSlash(fp.Path))
			if chartDir == "." {
				chartDir = ""
			}

			if !changedDirs[chartDir] && !changedDirs["."] {
				continue
			}

			var node yaml.Node
			if err := yaml.Unmarshal(fp.Content, &node); err != nil {
				return fmt.Errorf("failed to parse %s: %w", fp.Path, err)
			}

			var versionNode *yaml.Node
			if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
				root := node.Content[0]
				for j := 0; j < len(root.Content); j += 2 {
					if root.Content[j].Value == "version" {
						versionNode = root.Content[j+1]
						break
					}
				}
			}

			if versionNode != nil {
				newVer, err := bumpSemver(versionNode.Value, policy)
				if err != nil {
					return fmt.Errorf("failed to bump version in %s: %w", fp.Path, err)
				}
				versionNode.Value = newVer

				// encode back
				var buf bytes.Buffer
				enc := yaml.NewEncoder(&buf)
				enc.SetIndent(2)
				_ = enc.Encode(&node)
				enc.Close()

				p.Files[i].Content = buf.Bytes()

				if p.Files[i].Action == ActionUnchanged {
					p.Files[i].Action = ActionUpdated
				}
			}
		}
	}

	// Update dependencies
	for i, fp := range p.Files {
		if fp.Path == "Chart.yaml" {
			var node yaml.Node
			if err := yaml.Unmarshal(fp.Content, &node); err != nil {
				continue
			}

			updated := false
			if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
				root := node.Content[0]
				for j := 0; j < len(root.Content); j += 2 {
					if root.Content[j].Value == "dependencies" {
						deps := root.Content[j+1]
						if deps.Kind == yaml.SequenceNode {
							for _, dep := range deps.Content {
								var name string
								var versionNode *yaml.Node
								for k := 0; k < len(dep.Content); k += 2 {
									switch dep.Content[k].Value {
									case "name":
										name = dep.Content[k+1].Value
									case "version":
										versionNode = dep.Content[k+1]
									}
								}
								if name != "" && versionNode != nil {
									// Find the subchart
									subchartPath := path.Join("charts", name, "Chart.yaml")
									for _, sub := range p.Files {
										if filepath.ToSlash(sub.Path) == subchartPath {
											var subNode yaml.Node
											if yaml.Unmarshal(sub.Content, &subNode) == nil && len(subNode.Content) > 0 {
												subRoot := subNode.Content[0]
												for l := 0; l < len(subRoot.Content); l += 2 {
													if subRoot.Content[l].Value == "version" {
														if versionNode.Value != subRoot.Content[l+1].Value {
															versionNode.Value = subRoot.Content[l+1].Value
															updated = true
														}
														break
													}
												}
											}
											break
										}
									}
								}
							}
						}
					}
				}
			}

			if updated {
				var buf bytes.Buffer
				enc := yaml.NewEncoder(&buf)
				enc.SetIndent(2)
				_ = enc.Encode(&node)
				enc.Close()
				p.Files[i].Content = buf.Bytes()

				if p.Files[i].Action == ActionUnchanged {
					p.Files[i].Action = ActionUpdated
				}
			}
		}
	}

	return nil
}
