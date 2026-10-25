package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
)

func TestArgoCDApplicationProcessor(t *testing.T) {
	p := NewArgoCDApplicationProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	tests := []struct {
		name        string
		obj         *unstructured.Unstructured
		wantErr     bool
		expected    map[string]interface{}
		templateStr string
	}{
		{
			name: "Basic Application",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "argoproj.io/v1alpha1",
					"kind":       "Application",
					"metadata": map[string]interface{}{
						"name":      "test-app",
						"namespace": "argocd",
						"annotations": map[string]interface{}{
							"argocd.argoproj.io/sync-wave": "5",
							"argocd.argoproj.io/hook":      "PreSync",
							"other":                        "ignored",
						},
					},
					"spec": map[string]interface{}{
						"project": "default",
						"source": map[string]interface{}{
							"repoURL":        "https://github.com/example/repo.git",
							"path":           "apps/test",
							"targetRevision": "HEAD",
						},
						"destination": map[string]interface{}{
							"server":    "https://kubernetes.default.svc",
							"namespace": "test",
						},
						"syncPolicy": map[string]interface{}{
							"automated": map[string]interface{}{
								"prune":    true,
								"selfHeal": true,
							},
						},
					},
				},
			},
			wantErr: false,
			expected: map[string]interface{}{
				"project": "default",
			},
			templateStr: "apiVersion: argoproj.io/v1alpha1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Process(ctx, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			testutil.AssertEqual(t, true, result.Processed)
			testutil.AssertEqual(t, "test-app", result.ServiceName)
			testutil.AssertEqual(t, "templates/test-app-application.yaml", result.TemplatePath)
			testutil.AssertEqual(t, "services.test-app.argoApplication", result.ValuesPath)

			testutil.AssertEqual(t, tt.expected["project"], result.Values["project"])

			if tt.templateStr != "" && !strings.Contains(result.TemplateContent, tt.templateStr) {
				t.Errorf("Expected template to contain %q", tt.templateStr)
			}
		})
	}
}

func TestArgoCDApplicationSetProcessor(t *testing.T) {
	p := NewArgoCDApplicationSetProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	tests := []struct {
		name        string
		obj         *unstructured.Unstructured
		wantErr     bool
		expected    map[string]interface{}
		templateStr string
	}{
		{
			name: "Basic ApplicationSet",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "argoproj.io/v1alpha1",
					"kind":       "ApplicationSet",
					"metadata": map[string]interface{}{
						"name":      "test-appset",
						"namespace": "argocd",
						"annotations": map[string]interface{}{
							"argocd.argoproj.io/sync-wave": "10",
						},
					},
					"spec": map[string]interface{}{
						"generators": []interface{}{
							map[string]interface{}{
								"list": map[string]interface{}{
									"elements": []interface{}{
										map[string]interface{}{"cluster": "cluster1", "url": "https://1.2.3.4"},
									},
								},
							},
						},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"name": "{{cluster}}-guestbook",
							},
							"spec": map[string]interface{}{
								"project": "default",
								"source": map[string]interface{}{
									"repoURL": "https://github.com/argoproj/argocd-example-apps.git",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			expected: map[string]interface{}{
				"sync-wave": "10",
			},
			templateStr: "kind: ApplicationSet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Process(ctx, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			testutil.AssertEqual(t, true, result.Processed)
			testutil.AssertEqual(t, "test-appset", result.ServiceName)
			testutil.AssertEqual(t, "templates/test-appset-applicationset.yaml", result.TemplatePath)
			testutil.AssertEqual(t, "services.test-appset.argoApplicationSet", result.ValuesPath)

			if expectedSyncWave, ok := tt.expected["sync-wave"]; ok {
				annotations := result.Values["annotations"].(map[string]string)
				testutil.AssertEqual(t, expectedSyncWave, annotations["argocd.argoproj.io/sync-wave"])
			}

			if tt.templateStr != "" && !strings.Contains(result.TemplateContent, tt.templateStr) {
				t.Errorf("Expected template to contain %q", tt.templateStr)
			}
		})
	}
}
