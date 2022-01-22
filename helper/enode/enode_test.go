package enode

import (
	"fmt"
	"testing"
)

func TestParseEnode(t *testing.T) {
	id1 := "1dd9d65c4552b5eb43d5ad55a2ee3f56c6cbc1c64a5c8d659f51fcd51bace24351232b8d7821617d2b29b54b81cdefb9b3e9c37d7fd5f63270bcc9e1a6f6a439" //nolint:lll

	enode := func(prefix, id, ip, port string) string {
		return fmt.Sprintf("%s://%s@%s:%s", prefix, id, ip, port)
	}

	cases := []struct {
		Name  string
		enode string
		err   bool
	}{
		{
			Name:  "Incorrect scheme",
			enode: "foo://1234",
			err:   true,
		},
		{
			Name:  "Incorrect IP",
			enode: enode("enode", id1, "abc", "30303"),
			err:   true,
		},
		{
			Name:  "IP too long",
			enode: enode("enode", id1, "127.0.0.1.1", "30303"),
			err:   true,
		},
		{
			Name:  "IP too short",
			enode: enode("enode", id1, "127.0.0", "30303"),
			err:   true,
		},
		{
			Name:  "ID with 0x prefix",
			enode: enode("enode", "0x"+id1, "127.0.0.1.1", "30303"),
			err:   true,
		},
		{
			Name:  "ID is not hex",
			enode: enode("enode", "abcd", "127.0.0.1", "30303"),
			err:   true,
		},
		{
			Name:  "ID incorrect size",
			enode: enode("enode", id1[0:10], "127.0.0.1", "30303"),
			err:   true,
		},
		{
			Name:  "Port is not a number",
			enode: enode("enode", id1, "127.0.0.1", "aa"),
			err:   true,
		},
		{
			Name:  "Valid enode",
			enode: enode("enode", id1, "127.0.0.1", "30303"),
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			node, err := ParseURL(c.enode)
			if c.err && err == nil {
				t.Fatal("expected error")
			} else if !c.err && err != nil {
				t.Fatal("error not expected")
			}

			if err == nil {
				if node.String() != c.enode {
					t.Fatalf("bad")
				}
			}
		})
	}
}
