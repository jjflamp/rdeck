// Package tree builds the namespace tree from scanned keys — the Go
// counterpart of RESP.app's keysrendering / namespace grouping (the original
// used a Lua script; here grouping is done client-side per plan §2.3-②).
package tree

import (
	"sort"
	"strings"
)

type Node struct {
	Name        string  `json:"name"`              // last path segment
	FullPath    string  `json:"fullPath"`          // full key or namespace path
	IsNamespace bool    `json:"isNamespace"`
	KeyType     string  `json:"keyType,omitempty"` // "" for namespaces
	Count       int     `json:"count,omitempty"`   // namespace: total keys beneath
	Children    []*Node `json:"children,omitempty"`
}

// Input key list (key.Name used; Type attached to leaves).
type KeyInput struct {
	Name string
	Type string
}

// Build groups keys into a namespace tree split by sep (e.g. ":").
// A key whose full path is also a prefix of other keys ("a:b" key + "a:b:c"
// keys) becomes a namespace with the key attached as a pseudo-leaf.
// Children are sorted: namespaces first (by name), then keys (by name).
// Always returns a non-nil slice (nil would serialize as JSON null and break
// frontend rendering).
func Build(keys []KeyInput, sep string) []*Node {
	if keys == nil {
		keys = []KeyInput{}
	}
	if sep == "" {
		sep = ":"
	}

	// Pass 1: register every proper prefix of every key as a namespace path.
	nsPaths := map[string]bool{}
	for _, k := range keys {
		parts := strings.Split(k.Name, sep)
		for i := 1; i < len(parts); i++ {
			nsPaths[strings.Join(parts[:i], sep)] = true
		}
	}

	root := &Node{}
	findNamespace := func(n *Node, fullPath string) *Node {
		for _, c := range n.Children {
			if c.IsNamespace && c.FullPath == fullPath {
				return c
			}
		}
		return nil
	}

	// Pass 2: walk each key down existing namespaces; attach as leaf.
	for _, k := range keys {
		parts := strings.Split(k.Name, sep)
		cur := root
		for i, p := range parts {
			full := strings.Join(parts[:i+1], sep)
			if ns := findNamespace(cur, full); ns != nil {
				ns.Count++
				if i == len(parts)-1 {
					// Key shares its path with a namespace: attach as a
					// pseudo-leaf rendered as the namespace itself.
					ns.Children = append(ns.Children, &Node{
						Name:     "\x00self",
						FullPath: k.Name,
						KeyType:  k.Type,
					})
					break
				}
				cur = ns
				continue
			}
			if i == len(parts)-1 {
				cur.Children = append(cur.Children, &Node{
					Name:     p,
					FullPath: k.Name,
					KeyType:  k.Type,
				})
			} else {
				// Should not happen (pass 1 registered it), but stay safe.
				ns := &Node{Name: p, FullPath: full, IsNamespace: true, Count: 1}
				cur.Children = append(cur.Children, ns)
				cur = ns
			}
		}
	}

	if root.Children == nil {
		root.Children = []*Node{}
	}
	sortChildren(root)
	return root.Children
}

func sortChildren(n *Node) {
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsNamespace != b.IsNamespace {
			return a.IsNamespace // namespaces first
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		sortChildren(c)
	}
}
