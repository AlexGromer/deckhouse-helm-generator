package k8s

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// Generated charts keep the identity of the input objects: names, selectors
// and pod labels. These tests feed manifests parsed from YAML, as the
// extractors do (numbers arrive as float64).

func objFromYAML(t *testing.T, manifest string) *unstructured.Unstructured {
	t.Helper()
	obj := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		t.Fatal(err)
	}
	return &unstructured.Unstructured{Object: obj}
}

func processYAML(t *testing.T, p processor.Processor, manifest string) *processor.Result {
	t.Helper()
	res, err := p.Process(processor.Context{ChartName: "app"}, objFromYAML(t, manifest))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func assertContainsAll(t *testing.T, tpl string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(tpl, want) {
			t.Errorf("template misses %q:\n%s", want, tpl)
		}
	}
}

func TestProcessors_KeepInputNames(t *testing.T) {
	cases := []struct {
		p        processor.Processor
		manifest string
		name     string
	}{
		{NewDeploymentProcessor(), "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web-api\n  labels: {app: web}\nspec:\n  selector: {matchLabels: {app: web}}\n  template:\n    metadata: {labels: {app: web}}\n    spec: {containers: [{name: c, image: nginx}]}\n", "web-api"},
		{NewStatefulSetProcessor(), "apiVersion: apps/v1\nkind: StatefulSet\nmetadata: {name: db}\nspec:\n  serviceName: db-headless\n  selector: {matchLabels: {app: db}}\n  template:\n    metadata: {labels: {app: db}}\n    spec: {containers: [{name: c, image: pg}]}\n", "db"},
		{NewDaemonSetProcessor(), "apiVersion: apps/v1\nkind: DaemonSet\nmetadata: {name: agent}\nspec:\n  selector: {matchLabels: {app: agent}}\n  template:\n    metadata: {labels: {app: agent}}\n    spec: {containers: [{name: c, image: a}]}\n", "agent"},
		{NewServiceProcessor(), "apiVersion: v1\nkind: Service\nmetadata: {name: web-svc, labels: {app: web}}\nspec: {selector: {app: web}, ports: [{port: 80}]}\n", "web-svc"},
		{NewConfigMapProcessor(), "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: web-config, labels: {app: web}}\ndata: {a: b}\n", "web-config"},
		{NewSecretProcessor(), "apiVersion: v1\nkind: Secret\nmetadata: {name: web-secret, labels: {app: web}}\ntype: Opaque\ndata: {a: Yg==}\n", "web-secret"},
		{NewIngressProcessor(), "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata: {name: web-ing, labels: {app: web}}\nspec:\n  rules: [{host: a.example.com, http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: web-svc, port: {number: 80}}}}]}}]\n", "web-ing"},
		{NewRoleProcessor(), "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata: {name: reader, labels: {app: web}}\nrules: []\n", "reader"},
		{NewRoleBindingProcessor(), "apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata: {name: reader-binding, labels: {app: web}}\nroleRef: {apiGroup: rbac.authorization.k8s.io, kind: Role, name: reader}\nsubjects: [{kind: ServiceAccount, name: web}]\n", "reader-binding"},
		{NewClusterRoleProcessor(), "apiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata: {name: \"system:web-reader\"}\nrules: []\n", `"system:web-reader"`},
		{NewHPAProcessor(), "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata: {name: web-hpa, labels: {app: web}}\nspec: {scaleTargetRef: {apiVersion: apps/v1, kind: Deployment, name: web-api}, maxReplicas: 3}\n", "web-hpa"},
		{NewPDBProcessor(), "apiVersion: policy/v1\nkind: PodDisruptionBudget\nmetadata: {name: web-pdb, labels: {app: web}}\nspec: {maxUnavailable: 1, selector: {matchLabels: {app: web}}}\n", "web-pdb"},
		{NewJobProcessor(), "apiVersion: batch/v1\nkind: Job\nmetadata: {name: migrate, labels: {app: web}}\nspec:\n  template:\n    spec: {restartPolicy: Never, containers: [{name: c, image: m}]}\n", "migrate"},
		{NewCronJobProcessor(), "apiVersion: batch/v1\nkind: CronJob\nmetadata: {name: nightly, labels: {app: web}}\nspec:\n  schedule: '0 1 * * *'\n  jobTemplate:\n    spec:\n      template:\n        spec: {containers: [{name: c, image: b}]}\n", "nightly"},
		{NewCertificateProcessor(), "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata: {name: web-cert, labels: {app: web}}\nspec: {secretName: web-tls}\n", "web-cert"},
	}
	for _, tc := range cases {
		res := processYAML(t, tc.p, tc.manifest)
		tpl := res.TemplateContent
		if !strings.Contains(tpl, "\n  name: "+tc.name+"\n") {
			t.Errorf("%s: metadata.name must be %s:\n%s", tc.p.Name(), tc.name, tpl)
		}
		if strings.Contains(tpl, ".fullname") {
			t.Errorf("%s: the name must not get the release prefix:\n%s", tc.p.Name(), tpl)
		}
	}
}

func TestHPAProcessor_ScaleTargetKeepsInputName(t *testing.T) {
	res := processYAML(t, NewHPAProcessor(), "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata: {name: web-hpa}\nspec: {scaleTargetRef: {apiVersion: apps/v1, kind: Deployment, name: web-api}, maxReplicas: 3}\n")
	assertContainsAll(t, res.TemplateContent, "    name: {{ .scaleTargetRef.name }}\n")
}

func TestWorkloads_SelectorAndPodLabelsFromInput(t *testing.T) {
	res := processYAML(t, NewDeploymentProcessor(), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: web
      tier: front
    matchExpressions:
      - {key: track, operator: NotIn, values: [canary]}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: web
        version: v2
    spec:
      containers: [{name: web, image: nginx}]
`)
	wantSelector := map[string]interface{}{
		"matchLabels":      map[string]interface{}{"app.kubernetes.io/name": "web", "tier": "front"},
		"matchExpressions": []interface{}{map[string]interface{}{"key": "track", "operator": "NotIn", "values": []interface{}{"canary"}}},
	}
	if !reflect.DeepEqual(res.Values["selector"], wantSelector) {
		t.Errorf("selector = %v, want %v", res.Values["selector"], wantSelector)
	}
	// A matchLabels entry missing from the pod labels is added to them, so
	// the rendered workload is valid.
	wantLabels := map[string]string{"app.kubernetes.io/name": "web", "version": "v2", "tier": "front"}
	if !reflect.DeepEqual(res.Values["podLabels"], wantLabels) {
		t.Errorf("podLabels = %v, want %v", res.Values["podLabels"], wantLabels)
	}
	assertContainsAll(t, res.TemplateContent,
		"  selector:\n    {{- toYaml .selector | nindent 4 }}\n",
		`{{- toYaml (merge (dict) (.podLabels | default dict) (include "app.labels" $ | fromYaml)) | nindent 8 }}`,
	)
}

func TestWorkloads_SelectorFallbacks(t *testing.T) {
	// No selector: the pod labels select the pods.
	res := processYAML(t, NewStatefulSetProcessor(), "apiVersion: apps/v1\nkind: StatefulSet\nmetadata: {name: db}\nspec:\n  template:\n    metadata: {labels: {app: db}}\n    spec: {containers: [{name: c, image: pg}]}\n")
	if want := map[string]interface{}{"matchLabels": map[string]interface{}{"app": "db"}}; !reflect.DeepEqual(res.Values["selector"], want) {
		t.Errorf("selector = %v, want %v", res.Values["selector"], want)
	}
	// Neither selector nor pod labels: app=<name>.
	res = processYAML(t, NewDaemonSetProcessor(), "apiVersion: apps/v1\nkind: DaemonSet\nmetadata: {name: agent}\nspec:\n  template:\n    spec: {containers: [{name: c, image: a}]}\n")
	if want := map[string]interface{}{"matchLabels": map[string]interface{}{"app": "agent"}}; !reflect.DeepEqual(res.Values["selector"], want) {
		t.Errorf("selector = %v, want %v", res.Values["selector"], want)
	}
	if want := map[string]string{"app": "agent"}; !reflect.DeepEqual(res.Values["podLabels"], want) {
		t.Errorf("podLabels = %v, want %v", res.Values["podLabels"], want)
	}
}

func TestServiceProcessor_SelectorFromInput(t *testing.T) {
	res := processYAML(t, NewServiceProcessor(), "apiVersion: v1\nkind: Service\nmetadata: {name: web}\nspec: {selector: {app.kubernetes.io/name: web}, ports: [{port: 80}]}\n")
	assertContainsAll(t, res.TemplateContent, "  {{- if .selector }}\n  selector:\n    {{- toYaml .selector | nindent 4 }}\n  {{- end }}\n")
	if strings.Contains(res.TemplateContent, "selectorLabels") {
		t.Errorf("chart selector labels must not replace the input's selector:\n%s", res.TemplateContent)
	}
	ext := processYAML(t, NewServiceProcessor(), "apiVersion: v1\nkind: Service\nmetadata: {name: db}\nspec: {type: ExternalName, externalName: db.example.com}\n")
	assertContainsAll(t, ext.TemplateContent, "  {{- with .externalName }}\n  externalName: {{ . }}\n")
}

func TestIngressProcessor_BackendPorts(t *testing.T) {
	res := processYAML(t, NewIngressProcessor(), `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: web}
spec:
  defaultBackend:
    resource: {apiGroup: k8s.example.com, kind: Bucket, name: static}
  rules:
    - host: a.example.com
      http:
        paths:
          - {path: /, pathType: Prefix, backend: {service: {name: web, port: {number: 80}}}}
          - {path: /api, pathType: Prefix, backend: {service: {name: api, port: {name: http}}}}
`)
	paths := res.Values["rules"].([]map[string]interface{})[0]["paths"].([]map[string]interface{})
	if got := paths[0]["service"].(map[string]interface{})["port"]; got != int64(80) {
		t.Errorf("port number parsed from YAML = %v (%T), want 80", got, got)
	}
	if got := paths[1]["service"].(map[string]interface{})["portName"]; got != "http" {
		t.Errorf("portName = %v", got)
	}
	if res.Values["defaultBackend"].(map[string]interface{})["resource"] == nil {
		t.Error("resource backend of defaultBackend dropped")
	}
	assertContainsAll(t, res.TemplateContent,
		"  defaultBackend:\n",
		"                {{- if .portName }}\n                port:\n                  name: {{ .portName }}\n                {{- else if .port }}\n                port:\n                  number: {{ .port }}\n                {{- end }}\n",
		"              {{- with .resource }}\n              resource:\n",
	)
}

func TestPodTemplate_SchedulingAndContainersForEveryWorkload(t *testing.T) {
	podSpec := `
      metadata:
        labels: {app: x}
        annotations: {prometheus.io/scrape: "true"}
      spec:
        serviceAccountName: x-sa
        nodeSelector: {disk: ssd}
        tolerations: [{key: dedicated, operator: Exists}]
        affinity: {nodeAffinity: {}}
        topologySpreadConstraints: [{maxSkew: 1, topologyKey: zone, whenUnsatisfiable: DoNotSchedule, labelSelector: {matchLabels: {app: x}}}]
        volumes: [{name: cfg, configMap: {name: x-config}}]
        initContainers: [{name: init, image: busybox, command: [sh]}]
        containers:
          - name: main
            image: registry.example.com:5000/x:1.2
            imagePullPolicy: Always
            args: [--serve]
            env: [{name: A, value: b}]
            envFrom: [{configMapRef: {name: x-env}}]
            volumeMounts: [{name: cfg, mountPath: /etc/x}]
`
	indent := func(s string, n int) string {
		pad := strings.Repeat(" ", n)
		return strings.ReplaceAll(s, "\n      ", "\n"+pad)
	}
	manifests := map[string]struct {
		p        processor.Processor
		manifest string
	}{
		"Deployment":  {NewDeploymentProcessor(), "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: x}\nspec:\n  selector: {matchLabels: {app: x}}\n  template:" + indent(podSpec, 4)},
		"StatefulSet": {NewStatefulSetProcessor(), "apiVersion: apps/v1\nkind: StatefulSet\nmetadata: {name: x}\nspec:\n  selector: {matchLabels: {app: x}}\n  template:" + indent(podSpec, 4)},
		"DaemonSet":   {NewDaemonSetProcessor(), "apiVersion: apps/v1\nkind: DaemonSet\nmetadata: {name: x}\nspec:\n  selector: {matchLabels: {app: x}}\n  template:" + indent(podSpec, 4)},
		"Job":         {NewJobProcessor(), "apiVersion: batch/v1\nkind: Job\nmetadata: {name: x}\nspec:\n  template:" + indent(podSpec, 4)},
		"CronJob":     {NewCronJobProcessor(), "apiVersion: batch/v1\nkind: CronJob\nmetadata: {name: x}\nspec:\n  schedule: '* * * * *'\n  jobTemplate:\n    spec:\n      template:" + indent(podSpec, 8)},
	}
	for kind, m := range manifests {
		res := processYAML(t, m.p, m.manifest)
		v := res.Values
		for _, key := range []string{"podLabels", "podAnnotations", "serviceAccountName", "nodeSelector", "tolerations", "affinity", "topologySpreadConstraints", "volumes", "initContainers", "containers"} {
			if v[key] == nil {
				t.Errorf("%s: values miss %s", kind, key)
			}
		}
		c := v["containers"].([]map[string]interface{})[0]
		if img := c["image"].(map[string]interface{}); img["repository"] != "registry.example.com:5000/x" || img["tag"] != "1.2" || img["pullPolicy"] != "Always" {
			t.Errorf("%s: image = %v", kind, img)
		}
		for _, key := range []string{"args", "env", "envFrom", "volumeMounts"} {
			if c[key] == nil {
				t.Errorf("%s: container values miss %s", kind, key)
			}
		}
		tpl := res.TemplateContent
		for _, field := range []string{"nodeSelector", "tolerations", "affinity", "topologySpreadConstraints", "volumes", "initContainers"} {
			if !strings.Contains(tpl, "{{- with ."+field+" }}\n") {
				t.Errorf("%s: template does not render %s:\n%s", kind, field, tpl)
			}
		}
		for _, field := range []string{"env", "envFrom", "args", "volumeMounts"} {
			if !strings.Contains(tpl, "{{- with ."+field+" }}\n") {
				t.Errorf("%s: template does not render container %s", kind, field)
			}
		}
		assertContainsAll(t, tpl, "{{- with .podAnnotations }}", "(.podLabels | default dict)", "{{- with .serviceAccountName }}")
	}
}
