package detect

import (
	"go-s/internal/model"
	"math"
	"testing"
)

func TestMergeConfidenceAndDeterminism(t *testing.T) {
	tests := []struct {
		name    string
		signals []model.Signal
		members int
		want    float64
		root    bool
		member  []string
		layers  []int
	}{
		{name: "within scope corroboration", signals: []model.Signal{
			{Key: "node:react", Domain: "node", Layer: 2, Kind: model.ObsPackage, Member: "apps/web", Reason: "package", Evidence: []string{"package.json"}},
			{Key: "node:react", Domain: "node", Layer: 3, Kind: model.ObsConfig, Member: "apps/web", Reason: "config", Evidence: []string{"next.config.js"}},
		}, members: 1, want: 1.0, member: []string{"apps/web"}, layers: []int{2, 3}},
		{name: "partial member support", signals: []model.Signal{
			{Key: "go:module", Domain: "go", Layer: 2, Kind: model.ObsPackage, Member: "one"},
		}, members: 2, want: 0.7125, member: []string{"one"}, layers: []int{2}},
		{name: "root only", signals: []model.Signal{
			{Key: "infra:docker", Domain: "infra", Layer: 3, Kind: model.ObsConfig, Member: "."},
		}, members: 3, want: 0.85, root: true, layers: []int{3}},
		{name: "root beats member penalty", signals: []model.Signal{
			{Key: "infra:docker", Domain: "infra", Layer: 3, Kind: model.ObsConfig, Member: "."},
			{Key: "infra:docker", Domain: "infra", Layer: 3, Kind: model.ObsConfig, Member: "one"},
		}, members: 3, want: 0.85, root: true, member: []string{"one"}, layers: []int{3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Merge(test.signals, test.members)
			if len(got) != 1 {
				t.Fatalf("got %d merged signals, want 1", len(got))
			}
			if math.Abs(got[0].Confidence-test.want) > 1e-9 {
				t.Fatalf("confidence = %v, want %v", got[0].Confidence, test.want)
			}
			if got[0].RootObserved != test.root {
				t.Fatalf("root observed = %v, want %v", got[0].RootObserved, test.root)
			}
			if len(got[0].Members) != len(test.member) {
				t.Fatalf("members = %#v, want %#v", got[0].Members, test.member)
			}
			for i := range test.member {
				if got[0].Members[i] != test.member[i] {
					t.Fatalf("members = %#v, want %#v", got[0].Members, test.member)
				}
			}
			if len(got[0].Layers) != len(test.layers) {
				t.Fatalf("layers = %#v, want %#v", got[0].Layers, test.layers)
			}
		})
	}
}

func TestMergeSortsAndUnionsEvidence(t *testing.T) {
	got := Merge([]model.Signal{
		{Key: "z", Domain: "z", Layer: 5, Member: ".", Reason: "b", Evidence: []string{"z"}},
		{Key: "a", Domain: "a", Layer: 2, Kind: model.ObsPackage, Member: ".", Reason: "a", Evidence: []string{"a"}},
		{Key: "z", Domain: "z", Layer: 5, Member: ".", Reason: "a", Evidence: []string{"a"}},
	}, 0)
	if got[0].Key != "a" || got[1].Key != "z" {
		t.Fatalf("unexpected order: %#v", got)
	}
	if len(got[1].Reasons) != 2 || got[1].Reasons[0] != "a" || got[1].Evidence[0] != "a" {
		t.Fatalf("unions not deterministic: %#v", got[1])
	}
}

func TestMergeStableUnionsAcrossMembers(t *testing.T) {
	signals := []model.Signal{{Key: "x", Domain: "z", Reason: "shared", Evidence: []string{"same"}, Layer: 2, Member: "a"}, {Key: "x", Domain: "a", Reason: "shared", Evidence: []string{"same"}, Layer: 2, Member: "b"}}
	got := Merge(signals, 2)
	if len(got) != 1 || len(got[0].Reasons) != 1 || len(got[0].Evidence) != 1 || got[0].Domain != "a" {
		t.Fatalf("noncanonical union: %+v", got)
	}
	for i := 0; i < 20; i++ {
		again := Merge(signals, 2)
		if again[0].Domain != got[0].Domain {
			t.Fatal("nondeterministic domain")
		}
	}
}
