package codex

import (
	"os"
	"strings"
	"testing"
)

func TestExtractAndMergeAuthConfigPreservesUnrelatedSections(t *testing.T) {
	source := writeConfig(t, "model = \"gpt-5\"\nmodel_provider = \"corp\"\nmodel_reasoning_effort = \"high\"\nmodel_reasoning_summary = \"detailed\"\nmodel_verbosity = \"low\"\nfoo = \"bar\"\n\n[model_providers.corp]\nbase_url = \"http://models\"\nenv_key = \"OPENAI_API_KEY\"\n\n[model_providers.corp.headers]\nX-Team = \"platform\"\n\n[model_providers.other]\nbase_url = \"http://other\"\n\n[mcp_servers.docs]\ncommand = \"npx\"\n")
	fragment, summary, err := ExtractAuthConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Provider != "corp" {
		t.Fatalf("summary: %#v", summary)
	}
	if strings.Contains(string(fragment), "model =") || strings.Contains(string(fragment), "model_reasoning_effort") || strings.Contains(string(fragment), "mcp_servers") || strings.Contains(string(fragment), "foo") || strings.Contains(string(fragment), "model_reasoning_summary") || strings.Contains(string(fragment), "model_verbosity") || strings.Contains(string(fragment), "model_providers.other") {
		t.Fatalf("leaked config:\n%s", fragment)
	}
	if !containsAll(string(fragment), "model_provider = \"corp\"", "[model_providers.corp.headers]", "X-Team = \"platform\"") {
		t.Fatalf("missing requested profile config:\n%s", fragment)
	}
	if !strings.Contains(string(fragment), "name = \"corp\"") {
		t.Fatalf("provider name missing from profile config:\n%s", fragment)
	}
	target := writeConfig(t, "model = \"old\"\nmodel_provider = \"oldp\"\nmodel_reasoning_effort = \"low\"\nmodel_reasoning_summary = \"keep-summary\"\nmodel_verbosity = \"keep-verbosity\"\nfoo = \"keep\"\n\n[model_providers.oldp]\nbase_url = \"old\"\n\n[model_providers.corp]\nbase_url = \"old-corp\"\n\n[model_providers.corp.headers]\nX-Team = \"old-team\"\n\n[model_providers.unrelated]\nbase_url = \"keep-other\"\n\n[mcp_servers.local]\ncommand = \"local\"\n\n[projects.demo]\ntrust_level = \"trusted\"\n")
	if err := MergeAuthConfig(target, fragment); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	got := string(data)
	if !containsAll(got, "model = \"old\"", "model_provider = \"corp\"", "model_reasoning_effort = \"low\"", "model_reasoning_summary = \"keep-summary\"", "model_verbosity = \"keep-verbosity\"", "foo = \"keep\"", "[mcp_servers.local]", "[projects.demo]", "[model_providers.oldp]", "[model_providers.unrelated]", "[model_providers.corp.headers]") {
		t.Fatalf("unexpected merge:\n%s", got)
	}
	if !strings.Contains(got, "old-corp") || !strings.Contains(got, "old-team") {
		t.Fatalf("existing provider subtree was modified:\n%s", got)
	}
}

func TestCompareProviderConfigDetectsOnlyProviderConflicts(t *testing.T) {
	fragment := []byte("model = \"profile\"\nmodel_provider = \"corp\"\n\n[model_providers.corp]\nbase_url = \"https://profile.example/v1\"\n\n[model_providers.corp.headers]\nX-Team = \"profile\"\n")
	target := writeConfig(t, "model = \"local\"\nmodel_provider = \"local\"\n\n[model_providers.corp]\nbase_url = \"https://local.example/v1\"\n\n[model_providers.corp.headers]\nX-Team = \"local\"\n\n[mcp_servers.docs]\ncommand = \"keep\"\n")
	conflict, description, err := CompareProviderConfig(target, fragment)
	if err != nil {
		t.Fatal(err)
	}
	if !conflict || !strings.Contains(description, "differs") {
		t.Fatalf("conflict=%v description=%q", conflict, description)
	}
	identical := writeConfig(t, "model = \"local\"\nmodel_provider = \"local\"\n\n[model_providers.corp.headers]\nX-Team = \"profile\"\n\n[model_providers.corp]\nbase_url = \"https://profile.example/v1\"\n\n[mcp_servers.docs]\ncommand = \"keep\"\n")
	conflict, description, err = CompareProviderConfig(identical, fragment)
	if err != nil || conflict || description != "" {
		t.Fatalf("identical provider reported conflict=%v description=%q err=%v", conflict, description, err)
	}
}

func TestMergeAuthConfigClearsMissingProfileEffortAndPreservesOtherRootSettings(t *testing.T) {
	source := []byte("model = \"profile-model\"\nmodel_provider = \"profile-provider\"\n\n[model_providers.profile-provider]\nbase_url = \"https://profile.example/v1\"\n")
	target := writeConfig(t, "model = \"current-model\"\nmodel_provider = \"current-provider\"\nmodel_reasoning_effort = \"high\"\nmodel_reasoning_summary = \"detailed\"\nmodel_verbosity = \"low\"\napproval_policy = \"on-request\"\n\n[model_providers.current-provider]\nbase_url = \"https://current.example/v1\"\n")
	if err := MergeAuthConfig(target, source); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !containsAll(got, "model = \"current-model\"", "model_provider = \"profile-provider\"", "model_reasoning_effort = \"high\"", "model_reasoning_summary = \"detailed\"", "model_verbosity = \"low\"", "approval_policy = \"on-request\"", "[model_providers.current-provider]") {
		t.Fatalf("unrelated config was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "model = \"current-model\"") || !strings.Contains(got, "model_provider = \"profile-provider\"") || !strings.Contains(got, "model_reasoning_effort = \"high\"") {
		t.Fatalf("model settings were unexpectedly switched:\n%s", got)
	}
	if !strings.Contains(got, "[model_providers.current-provider]") {
		t.Fatalf("unselected provider table was removed:\n%s", got)
	}
}

func TestEnsureProviderBearerTokenAddsMemoryUserKey(t *testing.T) {
	path := writeConfig(t, "model_provider = \"team-proxy\"\n\n[model_providers.team-proxy]\nname = \"team-proxy\"\nbase_url = \"https://memory.yprenewables.com/codex/default\"\nwire_api = \"responses\"\n")
	if err := EnsureProviderBearerToken(path, []byte(`{"OPENAI_API_KEY":"sk-mem-test"}`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `experimental_bearer_token = "sk-mem-test"`) {
		t.Fatalf("bearer token missing: %s", data)
	}
}

func TestEnsureProviderBearerTokenInFragmentPreservesProviderSubtree(t *testing.T) {
	fragment := []byte("model_provider = \"team-proxy\"\n\n[model_providers.team-proxy]\nname = \"team-proxy\"\nbase_url = \"https://memory.example/codex/default\"\nwire_api = \"responses\"\nrequest_max_retries = 2\n\n[model_providers.team-proxy.headers]\nX-Team = \"platform\"\n")
	got, err := EnsureProviderBearerTokenInFragment(fragment, []byte(`{"OPENAI_API_KEY":"sk-mem-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, expected := range []string{"request_max_retries = 2", "[model_providers.team-proxy.headers]", "X-Team = \"platform\"", "experimental_bearer_token = \"sk-mem-test\""} {
		if !strings.Contains(text, expected) {
			t.Fatalf("fragment lost %q: %s", expected, text)
		}
	}
}
