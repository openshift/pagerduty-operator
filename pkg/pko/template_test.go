package pko

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

// deployPkoDir returns the path to the deploy_pko directory relative to the repo root.
func deployPkoDir() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "deploy_pko")); err == nil {
			return filepath.Join(dir, "deploy_pko")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "deploy_pko"
}

// templateFuncMap provides the functions available in PKO gotmpls.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"toJson": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"quote": func(v interface{}) string {
			return fmt.Sprintf("%q", v)
		},
	}
}

// renderTemplate renders a gotmpl file with the given config data and returns the output.
func renderTemplate(t *testing.T, filename string, data map[string]interface{}) string {
	t.Helper()
	tmplPath := filepath.Join(deployPkoDir(), filename)
	content, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("failed to read template %s: %v", tmplPath, err)
	}

	tmpl, err := template.New(filename).Funcs(templateFuncMap()).Parse(string(content))
	if err != nil {
		t.Fatalf("failed to parse template %s: %v", filename, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to execute template %s: %v", filename, err)
	}
	return buf.String()
}

// parseYAMLDocument parses a single YAML document into a map.
func parseYAMLDocument(t *testing.T, data string) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("failed to parse YAML: %v\nContent:\n%s", err, data)
	}
	return result
}

// defaultConfig returns a production-like config with populated arrays.
func defaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"image":                            "quay.io/example/pagerduty-operator:test",
			"fedramp":                          "false",
			"acknowledgeTimeout":               21600,
			"resolveTimeout":                   0,
			"escalationPolicy":                 "PTEST123",
			"escalationPolicySilent":           "PSILENT1",
			"servicePrefix":                    "osd",
			"scaleTestEscalationPolicy":        "PSCALE01",
			"scaleTestServicePrefix":           "osd-scale-test",
			"serviceOrchestrationEnabled":      "true",
			"serviceOrchestrationRuleConfigmap": "osd-serviceorchestration",
			"alertGroupingType":                "time",
			"alertGroupingTimeout":             300,
			"silentAlertLegalEntityIds": []string{
				"1aV37K1VQv2zSStwSkdwBNOUBGI",
				"2Mo8exhgEA5ir1lsVypXUb9v902",
			},
			"scaleTestLegalEntityIds": []string{
				"1ep4WsPDKikDQCFLsi2yKdzH8xB",
			},
			"serviceOrchestrationRules":        `{"orchestration_path":{"catch_all":{"actions":{}},"sets":[{"id":"start","rules":[{"actions":{"suppress":true},"conditions":[{"expression":"event.severity matches 'Warning'"}]}]}],"type":"service"}}`,
			"rhInfraServiceOrchestrationRules": `{"orchestration_path":{"catch_all":{"actions":{}},"sets":[{"id":"start","rules":[{"actions":{"suppress":true},"conditions":[{"expression":"event.severity matches 'Warning'"}]},{"actions":{"priority":"PDPJP5Q"},"conditions":[]}]}],"type":"service"}}`,
		},
	}
}

// emptyArraysConfig returns a config with empty arrays for edge-case testing.
func emptyArraysConfig() map[string]interface{} {
	config := defaultConfig()
	inner := config["config"].(map[string]interface{})
	inner["silentAlertLegalEntityIds"] = []string{}
	inner["scaleTestLegalEntityIds"] = []string{}
	inner["servicePrefix"] = "osdint"
	return config
}

// fedrampConfig returns a config with fedramp enabled.
func fedrampConfig() map[string]interface{} {
	config := defaultConfig()
	config["config"].(map[string]interface{})["fedramp"] = "true"
	return config
}

// orchestrationDisabledConfig returns a config with service orchestration disabled.
func orchestrationDisabledConfig() map[string]interface{} {
	config := defaultConfig()
	inner := config["config"].(map[string]interface{})
	inner["serviceOrchestrationEnabled"] = "false"
	inner["serviceOrchestrationRules"] = "{}"
	inner["rhInfraServiceOrchestrationRules"] = "{}"
	return config
}

func TestDeploymentGotmpl(t *testing.T) {
	output := renderTemplate(t, "Deployment-pagerduty-operator.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	if doc["kind"] != "Deployment" {
		t.Errorf("expected kind=Deployment, got %v", doc["kind"])
	}

	metadata := doc["metadata"].(map[string]interface{})
	if metadata["name"] != "pagerduty-operator" {
		t.Errorf("expected name=pagerduty-operator, got %v", metadata["name"])
	}
	if metadata["namespace"] != "pagerduty-operator" {
		t.Errorf("expected namespace=pagerduty-operator, got %v", metadata["namespace"])
	}

	annotations := metadata["annotations"].(map[string]interface{})
	if annotations["package-operator.run/phase"] != "deploy" {
		t.Errorf("expected phase=deploy, got %v", annotations["package-operator.run/phase"])
	}

	if !strings.Contains(output, "quay.io/example/pagerduty-operator:test") {
		t.Error("expected config.image to be substituted in Deployment")
	}

	if !strings.Contains(output, "value: 'false'") {
		t.Error("expected config.fedramp to be substituted in FEDRAMP env var")
	}
}

func TestDeploymentGotmpl_Fedramp(t *testing.T) {
	output := renderTemplate(t, "Deployment-pagerduty-operator.yaml.gotmpl", fedrampConfig())

	if !strings.Contains(output, "value: 'true'") {
		t.Error("expected FEDRAMP env var to be 'true' when fedramp config is true")
	}
}

func TestPDIOsdGotmpl(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	if doc["kind"] != "PagerDutyIntegration" {
		t.Errorf("expected kind=PagerDutyIntegration, got %v", doc["kind"])
	}

	metadata := doc["metadata"].(map[string]interface{})
	if metadata["name"] != "osd" {
		t.Errorf("expected name=osd, got %v", metadata["name"])
	}

	annotations := metadata["annotations"].(map[string]interface{})
	if annotations["package-operator.run/phase"] != "integrations" {
		t.Errorf("expected phase=integrations, got %v", annotations["package-operator.run/phase"])
	}

	spec := doc["spec"].(map[string]interface{})

	if spec["escalationPolicy"] != "PTEST123" {
		t.Errorf("expected escalationPolicy=PTEST123, got %v", spec["escalationPolicy"])
	}
	if spec["servicePrefix"] != "osd" {
		t.Errorf("expected servicePrefix=osd, got %v", spec["servicePrefix"])
	}

	// Verify serviceOrchestration block is present
	orch := spec["serviceOrchestration"].(map[string]interface{})
	// The gotmpl renders enabled: {{ .config.serviceOrchestrationEnabled }} which YAML
	// may parse as boolean true or string "true" depending on the value
	if fmt.Sprintf("%v", orch["enabled"]) != "true" {
		t.Errorf("expected serviceOrchestration.enabled=true, got %v", orch["enabled"])
	}

	// Verify alertGroupingParameters block is present
	alertGrouping := spec["alertGroupingParameters"].(map[string]interface{})
	if alertGrouping["type"] != "time" {
		t.Errorf("expected alertGroupingParameters.type=time, got %v", alertGrouping["type"])
	}
}

func TestPDIOsdGotmpl_ArrayRendering(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd.yaml.gotmpl", defaultConfig())

	// scaleTestLegalEntityIds should render as JSON array
	if !strings.Contains(output, `["1ep4WsPDKikDQCFLsi2yKdzH8xB"]`) {
		t.Error("expected scaleTestLegalEntityIds to render as JSON array")
	}

	// silentAlertLegalEntityIds should render as JSON array
	if !strings.Contains(output, `["1aV37K1VQv2zSStwSkdwBNOUBGI","2Mo8exhgEA5ir1lsVypXUb9v902"]`) {
		t.Error("expected silentAlertLegalEntityIds to render as JSON array")
	}
}

func TestPDIOsdGotmpl_EmptyArrays(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd.yaml.gotmpl", emptyArraysConfig())

	// Empty arrays should render as []
	if !strings.Contains(output, "[]") {
		t.Error("expected empty arrays to render as []")
	}

	// Should still be valid YAML
	parseYAMLDocument(t, output)
}

func TestPDISilentGotmpl(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd-silent.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	metadata := doc["metadata"].(map[string]interface{})
	if metadata["name"] != "osd-silent" {
		t.Errorf("expected name=osd-silent, got %v", metadata["name"])
	}

	spec := doc["spec"].(map[string]interface{})

	// Should use escalationPolicySilent
	if spec["escalationPolicy"] != "PSILENT1" {
		t.Errorf("expected escalationPolicy=PSILENT1, got %v", spec["escalationPolicy"])
	}

	// servicePrefix should have -silent suffix
	if spec["servicePrefix"] != "osd-silent" {
		t.Errorf("expected servicePrefix=osd-silent, got %v", spec["servicePrefix"])
	}

	// Should NOT have serviceOrchestration block
	if _, ok := spec["serviceOrchestration"]; ok {
		t.Error("osd-silent should NOT have serviceOrchestration block")
	}

	// Should NOT have alertGroupingParameters block
	if _, ok := spec["alertGroupingParameters"]; ok {
		t.Error("osd-silent should NOT have alertGroupingParameters block")
	}
}

func TestPDISilentGotmpl_SilentIdsInValues(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd-silent.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	spec := doc["spec"].(map[string]interface{})
	selector := spec["clusterDeploymentSelector"].(map[string]interface{})
	expressions := selector["matchExpressions"].([]interface{})

	// Find the silent alert legal entity ID expression — should use operator: In
	for _, expr := range expressions {
		e := expr.(map[string]interface{})
		if e["key"] == "api.openshift.com/legal-entity-id" && e["operator"] == "In" {
			return // Found the expected In operator for silent IDs
		}
	}
	t.Error("expected legal-entity-id matchExpression with operator: In for silent alert IDs")
}

func TestPDIScaleTestGotmpl(t *testing.T) {
	output := renderTemplate(t, "PagerDutyIntegration-osd-scale-test.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	metadata := doc["metadata"].(map[string]interface{})
	if metadata["name"] != "osd-scale-test" {
		t.Errorf("expected name=osd-scale-test, got %v", metadata["name"])
	}

	spec := doc["spec"].(map[string]interface{})

	if spec["escalationPolicy"] != "PSCALE01" {
		t.Errorf("expected escalationPolicy=PSCALE01, got %v", spec["escalationPolicy"])
	}
	if spec["servicePrefix"] != "osd-scale-test" {
		t.Errorf("expected servicePrefix=osd-scale-test, got %v", spec["servicePrefix"])
	}

	// scaleTestLegalEntityIds should use operator: In
	selector := spec["clusterDeploymentSelector"].(map[string]interface{})
	expressions := selector["matchExpressions"].([]interface{})

	for _, expr := range expressions {
		e := expr.(map[string]interface{})
		if e["key"] == "api.openshift.com/legal-entity-id" && e["operator"] == "In" {
			return
		}
	}
	t.Error("expected legal-entity-id matchExpression with operator: In for scale test IDs")
}

func TestConfigMapGotmpl(t *testing.T) {
	output := renderTemplate(t, "ConfigMap-osd-serviceorchestration.yaml.gotmpl", defaultConfig())
	doc := parseYAMLDocument(t, output)

	if doc["kind"] != "ConfigMap" {
		t.Errorf("expected kind=ConfigMap, got %v", doc["kind"])
	}

	metadata := doc["metadata"].(map[string]interface{})
	if metadata["name"] != "osd-serviceorchestration" {
		t.Errorf("expected name=osd-serviceorchestration, got %v", metadata["name"])
	}

	annotations := metadata["annotations"].(map[string]interface{})
	if annotations["package-operator.run/phase"] != "integrations" {
		t.Errorf("expected phase=integrations, got %v", annotations["package-operator.run/phase"])
	}

	// Verify both data keys are present
	data := doc["data"].(map[string]interface{})
	if _, ok := data["service-orchestration.json"]; !ok {
		t.Error("expected data key 'service-orchestration.json'")
	}
	if _, ok := data["rh-infra-service-orchestration.json"]; !ok {
		t.Error("expected data key 'rh-infra-service-orchestration.json'")
	}

	// Verify JSON rules are present in the rendered output
	if !strings.Contains(output, "orchestration_path") {
		t.Error("expected service orchestration rules JSON to be rendered")
	}
}

func TestConfigMapGotmpl_EmptyRules(t *testing.T) {
	output := renderTemplate(t, "ConfigMap-osd-serviceorchestration.yaml.gotmpl", orchestrationDisabledConfig())

	// Should still be valid YAML
	parseYAMLDocument(t, output)

	// Empty rules should render as quoted empty JSON object
	if !strings.Contains(output, "{}") {
		t.Error("expected empty orchestration rules to render as {}")
	}
}

func TestAllGotmplsRenderWithDefaults(t *testing.T) {
	dir := deployPkoDir()
	matches, err := filepath.Glob(filepath.Join(dir, "*.gotmpl"))
	if err != nil {
		t.Fatalf("failed to glob gotmpls: %v", err)
	}

	if len(matches) == 0 {
		t.Fatal("no .gotmpl files found in deploy_pko/")
	}

	config := defaultConfig()
	for _, match := range matches {
		filename := filepath.Base(match)
		t.Run(filename, func(t *testing.T) {
			output := renderTemplate(t, filename, config)
			parseYAMLDocument(t, output)
		})
	}
}
