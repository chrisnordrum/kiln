package ast

// Stmt is one statement in an action's do or after block.
//
// The set is closed. Mutation is limited to set, new and del; the only way out
// of the program is send, which must name an effect declared in app.kiln.
type Stmt interface {
	Position() Pos
	StmtKind() string
}

// Set assigns one field of one record: `set Task[id].done = done`.
type Set struct {
	Pos
	Table string
	Key   Expr
	Field string
	Value Expr
}

func (*Set) StmtKind() string { return "set" }

// SetSession establishes session state: `set session.user = User[id]`. Without
// it no action could sign anyone in, and the session a guard reads would have
// no way to come into existence.
type SetSession struct {
	Pos
	Name  string
	Value Expr
}

func (*SetSession) StmtKind() string { return "set" }

// New inserts a record: `new Task project=project title=title`.
type New struct {
	Pos
	Table  string
	Fields []*Attr
}

func (*New) StmtKind() string { return "new" }

// Del removes a record: `del Task[id]`.
type Del struct {
	Pos
	Table string
	Key   Expr
}

func (*Del) StmtKind() string { return "del" }

// Send invokes a declared effect: `send email to=user.email subject="hi"`.
type Send struct {
	Pos
	Effect string
	Args   []*Attr
}

func (*Send) StmtKind() string { return "send" }

// Refresh re-runs the calling route's data block and patches the page.
type Refresh struct{ Pos }

func (*Refresh) StmtKind() string { return "refresh" }

// Goto redirects after the action completes.
type Goto struct {
	Pos
	Path string
}

func (*Goto) StmtKind() string { return "goto" }

// Toast shows a message after the action completes.
type Toast struct {
	Pos
	Msg string
}

func (*Toast) StmtKind() string { return "toast" }
