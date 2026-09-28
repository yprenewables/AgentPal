package authprofile

import "testing"

func TestComputeContentHashIgnoresFormatting(t *testing.T) {
	authA := []byte(`{"OPENAI_API_KEY":"sk-1","nested":{"b":"2","a":"1"}}`)
	authB := []byte("{\n  \"nested\": {\"a\": \"1\", \"b\": \"2\"},\n  \"OPENAI_API_KEY\": \"sk-1\"\n}\n")
	configA := []byte("model_provider = \"yangpu\"\n\n[model_providers.yangpu]\nbase_url = \"u\"\nname = \"yangpu\"\n")
	configB := []byte("# reordered and reformatted\nmodel_provider = \"yangpu\"\n\n[model_providers.yangpu]\nname  =  \"yangpu\"\nbase_url = \"u\"\n")
	hashA, err := ComputeContentHash(authA, configA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := ComputeContentHash(authB, configB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatalf("hashA=%q hashB=%q", hashA, hashB)
	}
	if len(hashA) < len("sha256:") || hashA[:len("sha256:")] != "sha256:" {
		t.Fatalf("unexpected prefix: %q", hashA)
	}
}

func TestComputeContentHashRejectsInvalidInput(t *testing.T) {
	if _, err := ComputeContentHash([]byte("not-json"), []byte("model_provider = \"x\"\n")); err == nil {
		t.Fatal("invalid auth json accepted")
	}
	if _, err := ComputeContentHash([]byte(`{}`), []byte("===")); err == nil {
		t.Fatal("invalid toml accepted")
	}
}

func TestValidateReservedName(t *testing.T) {
	for _, name := range []string{"yangpu", "Team-1", "a.b_c"} {
		if err := ValidateReservedName(name); err != nil {
			t.Fatalf("%q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a b", "a:b"} {
		if err := ValidateReservedName(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
}
