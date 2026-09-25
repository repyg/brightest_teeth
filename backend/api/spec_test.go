package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// Validate the source contract (including examples) and ensure the checked-in
// generated contract still describes the same API.
func TestOpenAPIContract(t *testing.T) {
	source, err := openapi3.NewLoader().LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Validate(context.Background()); err != nil {
		t.Fatalf("invalid source specification: %v", err)
	}
	embedded, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	if err := embedded.Validate(context.Background()); err != nil {
		t.Fatalf("invalid generated specification: %v", err)
	}
	sourceJSON, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	embeddedJSON, err := json.Marshal(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceJSON) != string(embeddedJSON) {
		for i := 0; i < len(sourceJSON) && i < len(embeddedJSON); i++ {
			if sourceJSON[i] != embeddedJSON[i] {
				t.Logf("first difference at %d: source=%s generated=%s", i, sourceJSON[i:min(i+180, len(sourceJSON))], embeddedJSON[i:min(i+180, len(embeddedJSON))])
				break
			}
		}
		t.Fatal("generated contract is stale: run go generate ./...")
	}
}
