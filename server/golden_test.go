package server

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// updateGolden regenerates the tools/list golden file instead of comparing.
// Run: go test ./server -run TestToolsListGolden -update
var updateGolden = flag.Bool("update", false, "update golden files")

// goldenTool is the stable, comparable projection of a registered MCP tool. It
// captures exactly the public surface an MCP client sees: the tool's name,
// description, and its input/output JSON schemas.
type goldenTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// TestToolsListGolden locks the full tool catalog — names, descriptions, and
// inferred JSON schemas — against a golden file. Tool schemas are a public
// contract; a diff here means the wire shape changed and the golden must be
// updated deliberately (go test ./server -run TestToolsListGolden -update).
func TestToolsListGolden(t *testing.T) {
	// SQL enabled so the golden also locks the opt-in query_sql tool's schema.
	cs := connectSQL(t, &fakeSource{freshness: sampleFreshness()})

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	tools := make([]goldenTool, 0, len(res.Tools))
	for _, tl := range res.Tools {
		gt := goldenTool{Name: tl.Name, Description: tl.Description}
		if tl.InputSchema != nil {
			gt.InputSchema = mustMarshalSchema(t, tl.InputSchema)
		}
		if tl.OutputSchema != nil {
			gt.OutputSchema = mustMarshalSchema(t, tl.OutputSchema)
		}
		tools = append(tools, gt)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	got, err := json.MarshalIndent(tools, "", "  ")
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", "tools_list.golden.json")
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("tools/list golden mismatch — the public tool contract changed.\n"+
			"If intentional, regenerate: go test ./server -run TestToolsListGolden -update\n\n--- got ---\n%s", got)
	}
}

func mustMarshalSchema(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	return b
}
