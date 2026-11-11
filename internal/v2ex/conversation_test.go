package v2ex

import (
	"reflect"
	"testing"
)

func TestConversation(t *testing.T) {
	replies := []Reply{
		{ID: 1, Floor: 1, Author: "alice", Content: "一个问题"},
		{ID: 2, Floor: 2, Author: "bob", Content: "另一个问题"},
		{ID: 3, Floor: 3, Author: "carol", Content: "@alice #1 回复第一个问题"},
		{ID: 4, Floor: 4, Author: "alice", Content: "@carol 谢谢"},
		{ID: 5, Floor: 5, Author: "dan", Content: "@alice #1 明确引用优先于最近用户名"},
		{ID: 6, Floor: 6, Author: "eve", Content: "@bob @carol 同时回复时选择最近前文"},
		{ID: 7, Floor: 7, Author: "frank", Content: "@nobody #99 找不到前文"},
	}
	got := Conversation(replies)
	var floors, parents []int
	seen := map[int]bool{}
	for _, r := range got {
		floors = append(floors, r.Floor)
		parents = append(parents, r.ParentFloor)
		if seen[r.ID] {
			t.Fatalf("reply duplicated: %d", r.ID)
		}
		seen[r.ID] = true
		if r.ParentFloor >= r.Floor && r.ParentFloor != 0 {
			t.Fatalf("forward edge: %+v", r)
		}
	}
	if !reflect.DeepEqual(floors, []int{1, 3, 4, 6, 5, 2, 7}) {
		t.Fatalf("floors: %v", floors)
	}
	if !reflect.DeepEqual(parents, []int{0, 1, 3, 3, 1, 0, 0}) {
		t.Fatalf("parents: %v", parents)
	}
	if got[1].Inferred || !got[2].Inferred || !got[3].Inferred {
		t.Fatal("explicit/inferred references not distinguished")
	}
	if replies[1].Floor != 2 {
		t.Fatal("input was mutated")
	}
}

func TestConversationPageBoundaryAndCycles(t *testing.T) {
	got := Conversation([]Reply{
		{ID: 3, Floor: 103, Author: "same", Content: "@same #102"},
		{ID: 1, Floor: 101, Author: "same", Content: "@missing #5"},
		{ID: 2, Floor: 102, Author: "same", Content: "#103"},
	})
	if len(got) != 3 || got[0].Floor != 101 || got[1].Floor != 102 || got[1].ParentFloor != 0 || got[2].ParentFloor != 102 {
		t.Fatalf("unexpected forest: %+v", got)
	}
}
