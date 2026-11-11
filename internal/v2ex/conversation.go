package v2ex

import (
	"regexp"
	"sort"
	"strings"
)

var floorReference = regexp.MustCompile(`(?:^|[\s，。,:：])#([1-9][0-9]*)\b`)
var mentionReference = regexp.MustCompile(`@([A-Za-z0-9_]+)`)

// Conversation is a deterministic forest over the replies loaded on this page.
// Explicit earlier floor references win; otherwise use the most recent mentioned
// author. A reply has exactly one parent and is rendered exactly once.
type ConversationReply struct {
	Reply
	Depth, ParentFloor int
	Inferred           bool
}

func Conversation(replies []Reply) []ConversationReply {
	ordered := append([]Reply(nil), replies...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Floor < ordered[j].Floor })
	byFloor, byAuthor := map[int]int{}, map[string]int{}
	children := make([][]int, len(ordered))
	parents := make([]int, len(ordered))
	inferred := make([]bool, len(ordered))
	var roots []int
	for i, r := range ordered {
		parent := -1
		for _, match := range floorReference.FindAllStringSubmatch(r.Content, -1) {
			if at, ok := byFloor[number(match[1])]; ok && (parent < 0 || ordered[at].Floor > ordered[parent].Floor) {
				parent = at
			}
		}
		if parent < 0 {
			for _, match := range mentionReference.FindAllStringSubmatch(r.Content, -1) {
				if at, ok := byAuthor[strings.ToLower(match[1])]; ok && (parent < 0 || at > parent) {
					parent = at
				}
			}
			inferred[i] = parent >= 0
		}
		parents[i] = parent
		if parent < 0 {
			roots = append(roots, i)
		} else {
			children[parent] = append(children[parent], i)
		}
		byFloor[r.Floor] = i
		byAuthor[strings.ToLower(r.Author)] = i
	}
	result := make([]ConversationReply, 0, len(ordered))
	var visit func(int, int)
	visit = func(i, depth int) {
		r := ConversationReply{Reply: ordered[i], Depth: depth, Inferred: inferred[i]}
		if parents[i] >= 0 {
			r.ParentFloor = ordered[parents[i]].Floor
		}
		result = append(result, r)
		for _, child := range children[i] {
			visit(child, depth+1)
		}
	}
	for _, root := range roots {
		visit(root, 0)
	}
	return result
}
