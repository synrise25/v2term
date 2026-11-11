package v2ex

// Page contains display data only. Credentials and CSRF tokens never enter UI state.
type Page struct {
	Path, Title, Username string
	Topics                []Topic
	Topic                 *Topic
	Replies               []Reply
	Number, Pages         int
	CanReply              bool
}

type Topic struct {
	ID                       int
	Title, Author, Node, Age string
	Path, NodePath, Content  string
	Replies                  int
	Body                     []Element
}

type Reply struct {
	ID, Floor, Thanks    int
	Author, Age, Content string
	Body                 []Element
}

// Element preserves inline link targets and image boundaries without footnotes.
type Element struct {
	Text, URL string
	Image     bool
}
