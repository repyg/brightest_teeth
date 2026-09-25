package mlapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestInternalContract(t *testing.T) {
	source, err := openapi3.NewLoader().LoadFromFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	embedded, err := GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	a, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("ML client is stale: run go generate ./...")
	}
}
