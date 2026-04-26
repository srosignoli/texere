package search_test

import (
	"os"
	"path/filepath"
	"testing"

	"texere/internal/content"
	"texere/internal/graph"
	"texere/internal/search"

	SST "github.com/srosignoli/sstorytime-kv/pkg/SSTorytime"
)

// TestSearchService_ContextFilter_OrbitRespectsScope locks in the fix for the
// kubectl/\context bug: when the same node text appears under two contexts,
// the builder dedupes it to a single node carrying both arrow links. A
// `\context "X"` query must therefore filter the orbit too — otherwise
// neighbors declared under sibling contexts leak into the result.
func TestSearchService_ContextFilter_OrbitRespectsScope(t *testing.T) {
	dir := t.TempDir()
	body := "" +
		"- Tools\n\n" +
		":: alpha ::\n\n" +
		"widget\n\"  (uses) alphaTarget\n\n" +
		":: beta ::\n\n" +
		"widget\n\"  (runs) betaTarget\n"
	if err := os.WriteFile(filepath.Join(dir, "tools.n4l"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	reg := &content.SSTRegistry{
		Arrows:      make(map[string]content.ArrowDef),
		Annotations: make(map[string]content.AnnotDef),
	}
	reg.Arrows["uses"] = content.ArrowDef{Code: "uses", LongName: "uses", MetaType: "express"}
	reg.Arrows["runs"] = content.ArrowDef{Code: "runs", LongName: "runs", MetaType: "express"}

	SST.NODE_DIRECTORY = SST.NodeDirectory{}
	SST.NODE_CACHE = make(map[SST.NodePtr]SST.NodePtr)
	if _, err := graph.BuildFromCollection(dir, reg); err != nil {
		t.Fatal(err)
	}

	svc := search.NewSearchService(dir, reg)
	t.Cleanup(svc.Close)

	res, err := svc.Search(`widget \context "alpha"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(res.Nodes))
	}
	n := res.Nodes[0]
	if n.NodeText != "widget" {
		t.Fatalf("want NodeText=widget, got %q", n.NodeText)
	}
	for _, o := range n.Orbit {
		if o.Ctx != "alpha" {
			t.Errorf("orbit leak: arrow %q -> %q with Ctx=%q must be filtered out by \\context \"alpha\"",
				o.ArrowLong, o.NeighborText, o.Ctx)
		}
		if o.NeighborText == "betaTarget" {
			t.Errorf("orbit contains betaTarget, which belongs to context \"beta\"")
		}
	}
}
