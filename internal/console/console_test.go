package console

import (
	"strings"
	"testing"
)

func TestSplitCommandString(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"SET key value", []string{"SET", "key", "value"}},
		{`SET key "hello world"`, []string{"SET", "key", "hello world"}},
		{`SET key 'a b'`, []string{"SET", "key", "a b"}},
		{"GET key\r\n", []string{"GET", "key"}},
		{`SET k "a\"b"`, []string{"SET", "k", `a"b`}},
		{"   ", nil},
		{`MSET a 1 b 2`, []string{"MSET", "a", "1", "b", "2"}},
	}
	for _, c := range cases {
		got, err := SplitCommandString(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: token %d = %q want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestSplitUnbalanced(t *testing.T) {
	if _, err := SplitCommandString(`SET k "abc`); err == nil {
		t.Fatal("expected unbalanced quote error")
	}
}

func TestRenderReply(t *testing.T) {
	if RenderReply(nil, 0) != "(nil)" {
		t.Fatal("nil")
	}
	if RenderReply(int64(5), 0) != "(integer) 5" {
		t.Fatal("int")
	}
	arr := []any{"a", []any{"b", "c"}}
	out := RenderReply(arr, 0)
	if !strings.Contains(out, "1) a") || !strings.Contains(out, "2)   1) b") {
		t.Fatalf("array render: %q", out)
	}
}
