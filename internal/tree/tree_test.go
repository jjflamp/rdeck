package tree

import (
	"reflect"
	"testing"
)

func TestBuildNamespaces(t *testing.T) {
	keys := []KeyInput{
		{Name: "user:1:profile", Type: "hash"},
		{Name: "user:1:settings", Type: "hash"},
		{Name: "user:2:profile", Type: "hash"},
		{Name: "session:abc", Type: "string"},
		{Name: "standalone", Type: "string"},
	}
	got := Build(keys, ":")

	// top level sorted: namespaces "session" (1), "user" (4), then key "standalone"
	if len(got) != 3 {
		t.Fatalf("want 3 top nodes, got %d: %+v", len(got), got)
	}
	if got[1].Name != "user" || got[1].Count != 3 || !got[1].IsNamespace {
		t.Fatalf("user namespace wrong: %+v", got[1])
	}
	if got[2].Name != "standalone" || got[2].IsNamespace || got[2].KeyType != "string" {
		t.Fatalf("leaf wrong: %+v", got[2])
	}

	// user has children 1 (ns, 2), 2 (ns, 1)
	u1 := got[1].Children[0]
	if u1.Name != "1" || u1.Count != 2 || len(u1.Children) != 2 {
		t.Fatalf("user:1 wrong: %+v", u1)
	}
	if u1.Children[0].FullPath != "user:1:profile" || u1.Children[0].KeyType != "hash" {
		t.Fatalf("leaf key wrong: %+v", u1.Children[0])
	}
}

func TestBuildKeyAndNamespaceSamePath(t *testing.T) {
	// "a:b" is both a key and a prefix of "a:b:c"
	keys := []KeyInput{
		{Name: "a:b:c", Type: "string"},
		{Name: "a:b", Type: "hash"},
	}
	got := Build(keys, ":")
	ns := got[0]
	if ns.Name != "a" || len(ns.Children) != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
	mid := ns.Children[0]
	if mid.Name != "b" || !mid.IsNamespace || mid.Count != 2 {
		t.Fatalf("shared path namespace wrong: %+v", mid)
	}
	if len(mid.Children) != 2 {
		t.Fatalf("expected self-attached key + child: %+v", mid.Children)
	}
	var leafTypes []string
	for _, c := range mid.Children {
		if !c.IsNamespace {
			leafTypes = append(leafTypes, c.KeyType)
		}
	}
	if !reflect.DeepEqual(leafTypes, []string{"hash", "string"}) {
		t.Fatalf("expected self-attached key + child: %+v", mid.Children)
	}
}

func TestBuildEmptySeparator(t *testing.T) {
	keys := []KeyInput{{Name: "plain", Type: "string"}}
	got := Build(keys, "")
	if len(got) != 1 || got[0].FullPath != "plain" {
		t.Fatalf(" %+v", got)
	}
}
