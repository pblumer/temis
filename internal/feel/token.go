package feel

// Kind enumerates the lexical token categories produced by the lexer.
//
// Per docs/30-feel-spec.md §2 the lexer does not assemble multi-word FEEL names:
// it emits one Name token per identifier fragment (and distinct tokens for
// keywords), leaving longest-match name assembly to the parser (WP-04).
type Kind int

// Token kinds.
const (
	EOF Kind = iota
	// Error marks an invalid lexeme. Its Value carries a human-readable message
	// and its Text the offending source. The lexer always makes progress after
	// an Error so tokenisation terminates on any input.
	Error

	// Literals and identifiers.
	Number // 42, 3.14, .5, 1.2e10
	String // "abc", with escapes resolved into Value
	At     // @"2024-01-01" temporal literal; Value holds the quoted content
	Name   // an identifier fragment

	// Keywords.
	And
	Or
	Not
	If
	Then
	Else
	For
	In
	Return
	Some
	Every
	Satisfies
	Between
	Instance
	Of
	Function
	External
	True
	False
	Null

	// Operators.
	Plus  // +
	Minus // -
	Star  // *
	Slash // /
	Pow   // **
	Eq    // =
	Neq   // !=
	Lt    // <
	Lte   // <=
	Gt    // >
	Gte   // >=

	// Punctuation.
	LParen   // (
	RParen   // )
	LBracket // [
	RBracket // ]
	LBrace   // {
	RBrace   // }
	Comma    // ,
	Colon    // :
	Dot      // .
	DotDot   // ..
)

// keywords maps reserved words to their token kind. true, false and null are
// included here as literal keywords.
var keywords = map[string]Kind{
	"and":       And,
	"or":        Or,
	"not":       Not,
	"if":        If,
	"then":      Then,
	"else":      Else,
	"for":       For,
	"in":        In,
	"return":    Return,
	"some":      Some,
	"every":     Every,
	"satisfies": Satisfies,
	"between":   Between,
	"instance":  Instance,
	"of":        Of,
	"function":  Function,
	"external":  External,
	"true":      True,
	"false":     False,
	"null":      Null,
}

// kindNames gives each kind a stable name for diagnostics and tests.
var kindNames = map[Kind]string{
	EOF: "EOF", Error: "Error",
	Number: "Number", String: "String", At: "At", Name: "Name",
	And: "and", Or: "or", Not: "not", If: "if", Then: "then", Else: "else",
	For: "for", In: "in", Return: "return", Some: "some", Every: "every",
	Satisfies: "satisfies", Between: "between", Instance: "instance", Of: "of",
	Function: "function", External: "external", True: "true", False: "false", Null: "null",
	Plus: "+", Minus: "-", Star: "*", Slash: "/", Pow: "**",
	Eq: "=", Neq: "!=", Lt: "<", Lte: "<=", Gt: ">", Gte: ">=",
	LParen: "(", RParen: ")", LBracket: "[", RBracket: "]", LBrace: "{", RBrace: "}",
	Comma: ",", Colon: ":", Dot: ".", DotDot: "..",
}

// String returns a stable name for the kind.
func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "Kind(?)"
}

// Token is a single lexical token with its source position (1-based line and
// column, measured in runes).
type Token struct {
	Kind Kind
	// Text is the exact source lexeme (raw, including quotes for strings).
	Text string
	// Value holds decoded content for String and At tokens (escapes resolved),
	// or the error message for Error tokens. It is empty otherwise.
	Value string
	Line  int
	Col   int
}
