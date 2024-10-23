// Taken from https://eli.thegreenplace.net/2022/a-faster-lexer-in-go/
package parse

import (
	"fmt"
	"io"
	"log"
	"unicode/utf8"

	"github.com/cptaffe/mailrules/rules"
)

// TokenType is a type for describing tokens mnemonically.
type TokenType int

// Token represents a single token in the input stream.
type Token struct {
	Type     TokenType
	Value    string
	Position int
}

// Values for TokenName
const (
	// Special tokens
	TokenError TokenType = iota
	TokenEOF

	TokenComment
	TokenIdentifier
	TokenNumber
	TokenQuote

	// Operators
	TokenPlus
	TokenMinus
	TokenMultiply
	TokenDivide
	TokenPeriod
	TokenBackslash
	TokenColon
	TokenPercent
	TokenPipe
	TokenExclamation
	TokenQuestion
	TokenPound
	TokenAmpersand
	TokenSemi
	TokenComma
	TokenLeftParen
	TokenRightParen
	TokenLeftAngle
	TokenRightAngle
	TokenLeftBrace
	TokenRightBrace
	TokenLeftBracket
	TokenRightBracket
	TokenEquals
	TokenTilde

	// Reserved words
	TokenIf
	TokenMove
	TokenAnd
	TokenOr
	TokenNot
	TokenThen
	TokenFlag
	TokenUnflag
	TokenStream
)

var tokenNames = [...]string{
	TokenError:        "ERROR",
	TokenEOF:          "EOF",
	TokenComment:      "COMMENT",
	TokenIdentifier:   "IDENTIFIER",
	TokenNumber:       "NUMBER",
	TokenQuote:        "QUOTE",
	TokenPlus:         "PLUS",
	TokenMinus:        "MINUS",
	TokenMultiply:     "MULTIPLY",
	TokenDivide:       "DIVIDE",
	TokenPeriod:       "PERIOD",
	TokenBackslash:    "BACKSLASH",
	TokenColon:        "COLON",
	TokenPercent:      "PERCENT",
	TokenPipe:         "PIPE",
	TokenExclamation:  "EXCLAMATION",
	TokenQuestion:     "QUESTION",
	TokenPound:        "POUND",
	TokenAmpersand:    "AMPERSAND",
	TokenSemi:         "SEMI",
	TokenComma:        "COMMA",
	TokenLeftParen:    "L_PAREN",
	TokenRightParen:   "R_PAREN",
	TokenLeftAngle:    "L_ANG",
	TokenRightAngle:   "R_ANG",
	TokenLeftBrace:    "L_BRACE",
	TokenRightBrace:   "R_BRACE",
	TokenLeftBracket:  "L_BRACKET",
	TokenRightBracket: "R_BRACKET",
	TokenEquals:       "EQUALS",
	TokenTilde:        "TILDE",
	TokenIf:           "IF",
	TokenMove:         "MOVE",
	TokenAnd:          "AND",
	TokenOr:           "OR",
	TokenNot:          "NOT",
	TokenThen:         "THEN",
	TokenFlag:         "FLAG",
	TokenUnflag:       "UNFLAG",
	TokenStream:       "STREAM",
}

var reservedWords = map[string]TokenType{
	"if":     TokenIf,
	"move":   TokenMove,
	"and":    TokenAnd,
	"or":     TokenOr,
	"not":    TokenNot,
	"then":   TokenThen,
	"flag":   TokenFlag,
	"unflag": TokenUnflag,
	"stream": TokenStream,
}

func (tok Token) String() string {
	return fmt.Sprintf("Token{%s, '%s', %d}", tokenNames[tok.Type], tok.Value, tok.Position)
}

func makeErrorToken(pos int) Token {
	return Token{TokenError, "", pos}
}

// Operator table for lookups.
var opTable = [...]TokenType{
	'+':  TokenPlus,
	'-':  TokenMinus,
	'*':  TokenMultiply,
	'/':  TokenDivide,
	'.':  TokenPeriod,
	'\\': TokenBackslash,
	':':  TokenColon,
	'%':  TokenPercent,
	'|':  TokenPipe,
	'!':  TokenExclamation,
	'?':  TokenQuestion,
	'#':  TokenPound,
	'&':  TokenAmpersand,
	';':  TokenSemi,
	',':  TokenComma,
	'(':  TokenLeftParen,
	')':  TokenRightParen,
	'<':  TokenLeftAngle,
	'>':  TokenRightAngle,
	'{':  TokenLeftBrace,
	'}':  TokenRightBrace,
	'[':  TokenLeftBracket,
	']':  TokenRightBracket,
	'=':  TokenEquals,
	'~':  TokenTilde,
}

// Lexer
//
// Create a new lexer with NewLexer and then call NextToken repeatedly to get
// tokens from the stream. The lexer will return a token with the name EOF when
// done.
type Lexer struct {
	buf []byte

	// Current rune.
	r rune

	// Position of the current rune in buf.
	rpos int

	// Position of the next rune in buf.
	nextpos int
}

// NewLexer creates a new lexer for the given input.
func NewLexer(buf []byte) *Lexer {
	lex := Lexer{buf, -1, 0, 0}

	// Prime the lexer by calling .next
	lex.next()
	return &lex
}

func (lex *Lexer) NextToken() Token {
	// Skip non-tokens like whitespace and check for EOF.
	lex.skipNontokens()
	if lex.r < 0 {
		return Token{TokenEOF, "", lex.nextpos}
	}

	// Is this an operator?
	if int(lex.r) < len(opTable) {
		if opName := opTable[lex.r]; opName != TokenError {
			if opName == TokenDivide {
				// Special case: '/' may be the start of a comment.
				if lex.peekNextByte() == '/' {
					return lex.scanComment()
				}
			}
			startpos := lex.rpos
			lex.next()
			return Token{opName, string(lex.buf[startpos:lex.rpos]), startpos}
		}
	}

	// Not an operator. Try other types of tokens.
	if isAlpha(lex.r) {
		return lex.scanIdentifier()
	} else if isDigit(lex.r) {
		return lex.scanNumber()
	} else if lex.r == '"' {
		return lex.scanQuote()
	}

	return makeErrorToken(lex.rpos)
}

// next advances the lexer's internal state to point to the next rune in the
// input.
func (lex *Lexer) next() {
	if lex.nextpos < len(lex.buf) {
		lex.rpos = lex.nextpos

		// r is the current rune, w is its width. We start by assuming the
		// common case - that the current rune is ASCII (and thus has width=1).
		r, w := rune(lex.buf[lex.nextpos]), 1

		if r >= utf8.RuneSelf {
			// The current rune is not actually ASCII, so we have to decode it
			// properly.
			r, w = utf8.DecodeRune(lex.buf[lex.nextpos:])
		}

		lex.nextpos += w
		lex.r = r
	} else {
		lex.rpos = len(lex.buf)
		lex.r = -1 // EOF
	}
}

// peekNextByte returns the next byte in the stream (the one after lex.r).
// Note: a single byte is peeked at - if there's a rune longer than a byte
// there, only its first byte is returned.
func (lex *Lexer) peekNextByte() rune {
	if lex.nextpos < len(lex.buf) {
		return rune(lex.buf[lex.nextpos])
	} else {
		return -1
	}
}

func (lex *Lexer) skipNontokens() {
	for lex.r == ' ' || lex.r == '\t' || lex.r == '\n' || lex.r == '\r' {
		lex.next()
	}
}

func (lex *Lexer) scanIdentifier() Token {
	startpos := lex.rpos
	for isAlpha(lex.r) || isDigit(lex.r) {
		lex.next()
	}
	val := string(lex.buf[startpos:lex.rpos])
	if typ, ok := reservedWords[val]; ok {
		return Token{typ, val, startpos}
	}
	return Token{TokenIdentifier, val, startpos}
}

func (lex *Lexer) scanNumber() Token {
	startpos := lex.rpos
	for isDigit(lex.r) {
		lex.next()
	}
	return Token{TokenNumber, string(lex.buf[startpos:lex.rpos]), startpos}
}

func (lex *Lexer) scanQuote() Token {
	startpos := lex.rpos
	lex.next()
	for lex.r > 0 && lex.r != '"' {
		if lex.r == '\\' {
			lex.next()
			switch lex.r {
			case '\\', '"':
			default:
				return makeErrorToken(lex.rpos)
			}
		}
		lex.next()
	}

	if lex.r < 0 {
		return makeErrorToken(startpos)
	} else {
		lex.next()
		return Token{TokenQuote, string(lex.buf[startpos:lex.rpos]), startpos}
	}
}

func (lex *Lexer) scanComment() Token {
	startpos := lex.rpos
	lex.next()
	for lex.r > 0 && lex.r != '\n' {
		lex.next()
	}

	tok := Token{TokenComment, string(lex.buf[startpos:lex.rpos]), startpos}
	lex.next()
	return tok
}

func isAlpha(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_' || r == '$'
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

func Parse(input io.Reader) ([]rules.Rule, error) {
	buf, err := io.ReadAll(input)
	if err != nil {
		log.Fatal(err)
	}

	lex := NewLexer(buf)
	parse := NewParser(lex)
	return parse.Parse()
}
