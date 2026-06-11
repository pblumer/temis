package feel

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// eof is the sentinel rune returned past the end of input.
const eof = rune(-1)

// Lexer turns FEEL source into a stream of tokens. It never panics: malformed
// input yields Error tokens and the lexer always advances, so Tokenize
// terminates on any input (docs/30-feel-spec.md §2, WP-03 acceptance criterion).
type Lexer struct {
	src  string
	pos  int // byte offset of the next rune
	line int
	col  int
}

// New returns a Lexer over src.
func New(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 1}
}

// Tokenize lexes the whole input and returns all tokens including the final EOF.
func Tokenize(src string) []Token {
	l := New(src)
	var toks []Token
	for {
		t := l.Next()
		toks = append(toks, t)
		if t.Kind == EOF {
			return toks
		}
	}
}

// Next returns the next token. At end of input it repeatedly returns EOF.
func (l *Lexer) Next() Token {
	l.skipSpace()
	line, col := l.line, l.col
	r := l.peek()
	switch {
	case r == eof:
		return Token{Kind: EOF, Line: line, Col: col}
	case isDigit(r) || (r == '.' && isDigit(l.peekAt(1))):
		return l.scanNumber(line, col)
	case r == '"':
		return l.scanString(line, col)
	case r == '@':
		return l.scanAt(line, col)
	case isNameStart(r):
		return l.scanName(line, col)
	default:
		return l.scanOperator(line, col)
	}
}

func (l *Lexer) skipSpace() {
	for {
		r := l.peek()
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || (r != eof && unicode.IsSpace(r)) {
			l.advance()
			continue
		}
		return
	}
}

func (l *Lexer) scanNumber(line, col int) Token {
	start := l.pos
	for isDigit(l.peek()) {
		l.advance()
	}
	// Fractional part, but only if a digit follows the dot so that "1..10"
	// lexes as Number, DotDot, Number rather than swallowing the range.
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		l.advance() // '.'
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	// Optional exponent, only when well-formed (e [+-]? digits).
	if r := l.peek(); (r == 'e' || r == 'E') && l.exponentAhead() {
		l.advance() // e/E
		if s := l.peek(); s == '+' || s == '-' {
			l.advance()
		}
		for isDigit(l.peek()) {
			l.advance()
		}
	}
	text := l.src[start:l.pos]
	return Token{Kind: Number, Text: text, Line: line, Col: col}
}

// exponentAhead reports whether the input starting at the current 'e'/'E'
// forms a valid exponent, without consuming anything.
func (l *Lexer) exponentAhead() bool {
	i := l.pos
	_, sz := utf8.DecodeRuneInString(l.src[i:]) // skip e/E
	i += sz
	if i < len(l.src) {
		if r, s := utf8.DecodeRuneInString(l.src[i:]); r == '+' || r == '-' {
			i += s
		}
	}
	if i >= len(l.src) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(l.src[i:])
	return isDigit(r)
}

func (l *Lexer) scanString(line, col int) Token {
	start := l.pos
	l.advance() // opening quote
	value, ok, msg := l.scanQuoted()
	text := l.src[start:l.pos]
	if !ok {
		return Token{Kind: Error, Text: text, Value: msg, Line: line, Col: col}
	}
	return Token{Kind: String, Text: text, Value: value, Line: line, Col: col}
}

func (l *Lexer) scanAt(line, col int) Token {
	start := l.pos
	l.advance() // '@'
	if l.peek() != '"' {
		return Token{Kind: Error, Text: l.src[start:l.pos], Value: `expected '"' after '@'`, Line: line, Col: col}
	}
	l.advance() // opening quote
	value, ok, msg := l.scanQuoted()
	text := l.src[start:l.pos]
	if !ok {
		return Token{Kind: Error, Text: text, Value: msg, Line: line, Col: col}
	}
	return Token{Kind: At, Text: text, Value: value, Line: line, Col: col}
}

// scanQuoted reads the body of a string after the opening quote up to and
// including the closing quote, resolving escapes. It reports the decoded value,
// whether the literal was well-formed, and an error message otherwise.
func (l *Lexer) scanQuoted() (value string, ok bool, msg string) {
	var b strings.Builder
	for {
		r := l.advance()
		switch r {
		case eof:
			return "", false, "unterminated string literal"
		case '"':
			return b.String(), true, ""
		case '\\':
			if !l.scanEscape(&b) {
				return "", false, "invalid escape sequence"
			}
		default:
			b.WriteRune(r)
		}
	}
}

// scanEscape consumes the character(s) after a backslash and writes the decoded
// rune. It reports whether the escape was valid.
func (l *Lexer) scanEscape(b *strings.Builder) bool {
	switch e := l.advance(); e {
	case '"':
		b.WriteByte('"')
	case '\\':
		b.WriteByte('\\')
	case '/':
		b.WriteByte('/')
	case 'b':
		b.WriteByte('\b')
	case 'f':
		b.WriteByte('\f')
	case 'n':
		b.WriteByte('\n')
	case 'r':
		b.WriteByte('\r')
	case 't':
		b.WriteByte('\t')
	case 'u':
		return l.scanUnicodeEscape(b)
	default:
		return false
	}
	return true
}

func (l *Lexer) scanUnicodeEscape(b *strings.Builder) bool {
	var cp rune
	for i := 0; i < 4; i++ {
		d, ok := hexValue(l.advance())
		if !ok {
			return false
		}
		cp = cp<<4 | d
	}
	b.WriteRune(cp)
	return true
}

func (l *Lexer) scanName(line, col int) Token {
	start := l.pos
	l.advance() // name start
	for isNamePart(l.peek()) {
		l.advance()
	}
	text := l.src[start:l.pos]
	if kw, ok := keywords[text]; ok {
		return Token{Kind: kw, Text: text, Line: line, Col: col}
	}
	return Token{Kind: Name, Text: text, Line: line, Col: col}
}

func (l *Lexer) scanOperator(line, col int) Token {
	start := l.pos
	r := l.advance()
	tok := func(k Kind) Token { return Token{Kind: k, Text: l.src[start:l.pos], Line: line, Col: col} }

	switch r {
	case '+':
		return tok(Plus)
	case '-':
		return tok(Minus)
	case '*':
		if l.peek() == '*' {
			l.advance()
			return tok(Pow)
		}
		return tok(Star)
	case '/':
		return tok(Slash)
	case '=':
		return tok(Eq)
	case '!':
		if l.peek() == '=' {
			l.advance()
			return tok(Neq)
		}
		return Token{Kind: Error, Text: l.src[start:l.pos], Value: "unexpected '!'", Line: line, Col: col}
	case '<':
		if l.peek() == '=' {
			l.advance()
			return tok(Lte)
		}
		return tok(Lt)
	case '>':
		if l.peek() == '=' {
			l.advance()
			return tok(Gte)
		}
		return tok(Gt)
	case '(':
		return tok(LParen)
	case ')':
		return tok(RParen)
	case '[':
		return tok(LBracket)
	case ']':
		return tok(RBracket)
	case '{':
		return tok(LBrace)
	case '}':
		return tok(RBrace)
	case ',':
		return tok(Comma)
	case ':':
		return tok(Colon)
	case '.':
		if l.peek() == '.' {
			l.advance()
			return tok(DotDot)
		}
		return tok(Dot)
	default:
		return Token{Kind: Error, Text: l.src[start:l.pos], Value: "unexpected character", Line: line, Col: col}
	}
}

// peek returns the current rune without consuming it, or eof at end of input.
func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

// peekAt returns the rune n runes ahead of the current position, or eof.
func (l *Lexer) peekAt(n int) rune {
	i := l.pos
	for ; n > 0 && i < len(l.src); n-- {
		_, sz := utf8.DecodeRuneInString(l.src[i:])
		i += sz
	}
	if n > 0 || i >= len(l.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(l.src[i:])
	return r
}

// advance consumes and returns the current rune, updating line/column. Invalid
// UTF-8 decodes to utf8.RuneError with width 1, guaranteeing forward progress.
func (l *Lexer) advance() rune {
	if l.pos >= len(l.src) {
		return eof
	}
	r, sz := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += sz
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// isNameStart reports whether r may begin a FEEL name fragment.
func isNameStart(r rune) bool {
	return r == '_' || r == '?' || unicode.IsLetter(r)
}

// isNamePart reports whether r may continue a FEEL name fragment.
func isNamePart(r rune) bool {
	return isNameStart(r) || unicode.IsDigit(r)
}

func hexValue(r rune) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return r - '0', true
	case r >= 'a' && r <= 'f':
		return r - 'a' + 10, true
	case r >= 'A' && r <= 'F':
		return r - 'A' + 10, true
	default:
		return 0, false
	}
}
