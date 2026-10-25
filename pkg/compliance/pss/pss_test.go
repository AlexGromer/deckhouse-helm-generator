package pss

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseProfile(t *testing.T) {
	tests := []struct {
		input   string
		want    Profile
		wantErr bool
	}{
		{"Restricted", ProfileRestricted, false},
		{"baseline", ProfileBaseline, false},
		{"PRIVILEGED", ProfilePrivileged, false},
		{"bad", "", true},
	}

	for _, tt := range tests {
		got, err := ParseProfile(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseProfile(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseProfile(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestEvaluateWorkload_Basic(t *testing.T) {
	// Empty pod spec
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
					},
				},
			},
		},
	}

	report := EvaluateWorkload(obj, ProfileRestricted)
	if report.Profile != ProfileBaseline {
		t.Errorf("Empty pod spec should be Baseline, got %v", report.Profile)
	}

	restrictedViolationCount := 0
	for _, f := range report.Findings {
		if f.Profile == ProfileRestricted {
			restrictedViolationCount++
		}
	}
	if restrictedViolationCount != 4 {
		t.Errorf("Expected 4 restricted violations, got %d", restrictedViolationCount)
	}

	// Fully compliant restricted pod
	objRestricted := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"spec": map[string]interface{}{
				"securityContext": map[string]interface{}{
					"runAsNonRoot": true,
					"seccompProfile": map[string]interface{}{
						"type": "RuntimeDefault",
					},
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
						"securityContext": map[string]interface{}{
							"allowPrivilegeEscalation": false,
							"capabilities": map[string]interface{}{
								"drop": []interface{}{"ALL"},
							},
						},
					},
				},
			},
		},
	}

	reportRestricted := EvaluateWorkload(objRestricted, ProfileRestricted)
	if reportRestricted.Profile != ProfileRestricted {
		t.Errorf("Compliant pod should be Restricted, got %v", reportRestricted.Profile)
	}
	if len(reportRestricted.Findings) != 0 {
		t.Errorf("Expected 0 findings, got %v", reportRestricted.Findings)
	}
}

func TestEvaluateWorkload_BaselineViolations(t *testing.T) {
	tests := []struct {
		name     string
		specMut  func(spec map[string]interface{})
		wantFind string
	}{
		{
			name: "hostNetwork",
			specMut: func(spec map[string]interface{}) {
				spec["hostNetwork"] = true
			},
			wantFind: "Host Namespaces",
		},
		{
			name: "hostPath",
			specMut: func(spec map[string]interface{}) {
				spec["volumes"] = []interface{}{
					map[string]interface{}{
						"name": "d",
						"hostPath": map[string]interface{}{
							"path": "/x",
						},
					},
				}
			},
			wantFind: "HostPath Volumes",
		},
		{
			name: "capabilities add",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"capabilities": map[string]interface{}{
						"add": []interface{}{"NET_ADMIN"},
					},
				}
			},
			wantFind: "Capabilities",
		},
		{
			name: "hostPort",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["ports"] = []interface{}{
					map[string]interface{}{
						"containerPort": int64(80),
						"hostPort":      int64(8080),
					},
				}
			},
			wantFind: "Host Ports",
		},
		{
			name: "hostProbe",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["livenessProbe"] = map[string]interface{}{
					"httpGet": map[string]interface{}{
						"host": "10.0.0.1",
					},
				}
			},
			wantFind: "Host Probes / Lifecycle Hooks",
		},
		{
			name: "seccomp Unconfined",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"seccompProfile": map[string]interface{}{
						"type": "Unconfined",
					},
				}
			},
			wantFind: "Seccomp",
		},
		{
			name: "sysctls",
			specMut: func(spec map[string]interface{}) {
				spec["securityContext"] = map[string]interface{}{
					"sysctls": []interface{}{
						map[string]interface{}{
							"name":  "kernel.msgmax",
							"value": "1",
						},
					},
				}
			},
			wantFind: "Sysctls",
		},
		{
			name: "seLinuxOptions",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"seLinuxOptions": map[string]interface{}{
						"type": "spc_t",
					},
				}
			},
			wantFind: "SELinux",
		},
		{
			name: "seLinuxOptions user",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"seLinuxOptions": map[string]interface{}{
						"user": "u",
					},
				}
			},
			wantFind: "SELinux",
		},
		{
			name: "procMount Unmasked",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"procMount": "Unmasked",
				}
			},
			wantFind: "/proc Mount Type",
		},
		{
			name: "appArmorProfile",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"appArmorProfile": map[string]interface{}{
						"type": "Unconfined",
					},
				}
			},
			wantFind: "AppArmor",
		},
		{
			name: "windowsOptions hostProcess",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"windowsOptions": map[string]interface{}{
						"hostProcess": true,
					},
				}
			},
			wantFind: "HostProcess",
		},
		{
			name: "invalid type (string for bool)",
			specMut: func(spec map[string]interface{}) {
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"privileged": "yes",
				}
			},
			wantFind: "Privileged Containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "a",
								"image": "x",
							},
						},
					},
				},
			}
			spec := obj.Object["spec"].(map[string]interface{})
			tt.specMut(spec)

			report := EvaluateWorkload(obj, ProfileBaseline)
			if report.Profile != ProfilePrivileged {
				t.Errorf("Expected Privileged for %s, got %v", tt.name, report.Profile)
			}
			found := false
			for _, f := range report.Findings {
				if f.Control == tt.wantFind {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected finding %q, got: %v", tt.wantFind, report.Findings)
			}
		})
	}
}

func TestEvaluateWorkload_AppArmorAnnotation(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"container.apparmor.security.beta.kubernetes.io/a": "unconfined",
				},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
					},
				},
			},
		},
	}
	report := EvaluateWorkload(obj, ProfileBaseline)
	if report.Profile != ProfilePrivileged {
		t.Errorf("Expected Privileged for AppArmor annotation violation")
	}
}

func TestEvaluateWorkload_RestrictedViolations(t *testing.T) {
	tests := []struct {
		name     string
		specMut  func(spec map[string]interface{})
		wantFind string
	}{
		{
			name: "runAsNonRoot container false",
			specMut: func(spec map[string]interface{}) {
				spec["securityContext"] = map[string]interface{}{
					"runAsNonRoot": true,
				}
				spec["containers"].([]interface{})[0].(map[string]interface{})["securityContext"] = map[string]interface{}{
					"runAsNonRoot": false,
				}
			},
			wantFind: "Running as Non-root",
		},
		{
			name: "runAsUser 0",
			specMut: func(spec map[string]interface{}) {
				spec["securityContext"] = map[string]interface{}{
					"runAsUser": int64(0),
				}
			},
			wantFind: "Running as Non-root user",
		},
		{
			name: "volume nfs",
			specMut: func(spec map[string]interface{}) {
				spec["volumes"] = []interface{}{
					map[string]interface{}{
						"name": "d",
						"nfs": map[string]interface{}{
							"path": "/x",
						},
					},
				}
			},
			wantFind: "Volume Types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "Pod",
					"spec": map[string]interface{}{
						"securityContext": map[string]interface{}{
							"runAsNonRoot": true,
							"seccompProfile": map[string]interface{}{
								"type": "RuntimeDefault",
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "a",
								"image": "x",
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
									"capabilities": map[string]interface{}{
										"drop": []interface{}{"ALL"},
									},
								},
							},
						},
					},
				},
			}
			spec := obj.Object["spec"].(map[string]interface{})
			tt.specMut(spec)

			report := EvaluateWorkload(obj, ProfileRestricted)
			if report.Profile != ProfileBaseline {
				t.Errorf("Expected Baseline for %s, got %v", tt.name, report.Profile)
			}
			found := false
			for _, f := range report.Findings {
				if f.Control == tt.wantFind {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected finding %q, got: %v", tt.wantFind, report.Findings)
			}
		})
	}
}

func TestEvaluateWorkload_WindowsOS(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"spec": map[string]interface{}{
				"os": map[string]interface{}{
					"name": "windows",
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
					},
				},
			},
		},
	}

	report := EvaluateWorkload(obj, ProfileRestricted)
	// Linux-only controls should be skipped (Privilege Escalation, Seccomp, Capabilities Restricted)
	// Empty pod spec still fails Running as Non-root for Restricted
	for _, f := range report.Findings {
		if f.Control == "Privilege Escalation" || f.Control == "Seccomp" || f.Control == "Capabilities" {
			t.Errorf("Windows pod should skip Linux-only controls, but found: %v", f.Control)
		}
	}
}

func TestPodSpecExtractors(t *testing.T) {
	tests := []struct {
		kind  string
		setup func(*unstructured.Unstructured)
		want  bool
	}{
		{
			kind: "Pod",
			setup: func(u *unstructured.Unstructured) {
				u.Object["spec"] = map[string]interface{}{}
			},
			want: true,
		},
		{
			kind: "CronJob",
			setup: func(u *unstructured.Unstructured) {
				u.Object["spec"] = map[string]interface{}{
					"jobTemplate": map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{},
							},
						},
					},
				}
			},
			want: true,
		},
		{
			kind: "Deployment",
			setup: func(u *unstructured.Unstructured) {
				u.Object["spec"] = map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{},
					},
				}
			},
			want: true,
		},
		{
			kind: "ConfigMap",
			setup: func(u *unstructured.Unstructured) {
			},
			want: false,
		},
	}

	for _, tt := range tests {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": tt.kind,
			},
		}
		tt.setup(u)
		_, _, _, ok := PodSpec(u)
		if ok != tt.want {
			t.Errorf("PodSpec(%s) ok = %v, want %v", tt.kind, ok, tt.want)
		}
	}
}

func TestReadOnlyRootFilesystemDoesNotAffectProfile(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"spec": map[string]interface{}{
				"securityContext": map[string]interface{}{
					"runAsNonRoot": true,
					"seccompProfile": map[string]interface{}{
						"type": "RuntimeDefault",
					},
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
						"securityContext": map[string]interface{}{
							"allowPrivilegeEscalation": false,
							"readOnlyRootFilesystem":   false, // Doesn't matter
							"capabilities": map[string]interface{}{
								"drop": []interface{}{"ALL"},
							},
						},
					},
				},
			},
		},
	}
	report := EvaluateWorkload(obj, ProfileRestricted)
	if report.Profile != ProfileRestricted {
		t.Errorf("Compliant pod should be Restricted regardless of readOnlyRootFilesystem, got %v", report.Profile)
	}
}

func TestEvaluateResourcesAndChart(t *testing.T) {
	// Just to make sure deadcode doesn't complain about EvaluateResources and EvaluateChart
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "a",
						"image": "x",
					},
				},
			},
		},
	}

	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
		},
	}

	report := EvaluateResources([]*types.ProcessedResource{res}, ProfileRestricted)
	if len(report.Workloads) != 1 {
		t.Errorf("Expected 1 workload in report, got %d", len(report.Workloads))
	}

	chartReport := EvaluateChart(nil, nil, ProfileRestricted)
	if len(chartReport.Workloads) != 0 {
		t.Errorf("Expected 0 workloads in chart report, got %d", len(chartReport.Workloads))
	}
}
