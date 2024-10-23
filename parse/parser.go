package parse

//go:generate goyacc -o rules.go -xe rules.examples -pool rules.y
import (
	"fmt"

	"github.com/cptaffe/mailrules/rules"
)

var tokenNumbers = [...]int{
	TokenIdentifier: IDENTIFIER,
	TokenQuote:      QUOTE,
	TokenEquals:     EQUALS,
	TokenTilde:      TILDE,
	TokenSemi:       SEMICOLON,
	TokenIf:         IF,
	TokenMove:       MOVE,
	TokenAnd:        AND,
	TokenOr:         OR,
	TokenNot:        NOT,
	TokenThen:       THEN,
	TokenFlag:       FLAG,
	TokenUnflag:     UNFLAG,
	TokenStream:     STREAM,
	TokenLeftParen:  LPAREN,
	TokenRightParen: RPAREN,
}

type Parser struct {
	lexer  *Lexer
	last   Token
	result []rules.Rule
	err    error
}

func (p *Parser) Lex(lval *yySymType) int {
	for {
		tok := p.lexer.NextToken()
		p.last = tok

		switch tok.Type {
		case TokenEOF:
			return -1
		case TokenError:
			p.err = fmt.Errorf("lexing error: %s", tok.Value)
			return -1
		case TokenComment:
			continue // skip
		default:
			lval.Value = tok.Value
			return tokenNumbers[tok.Type]
		}
	}
}

func (p *Parser) Error(err string) {
	p.err = fmt.Errorf("%s near position %d", err, p.last.Position)
}

func (p *Parser) Parse() ([]rules.Rule, error) {
	yyParse(p)
	if p.err != nil {
		return nil, p.err
	}
	return p.result, nil
}

func NewParser(lexer *Lexer) *Parser {
	return &Parser{lexer: lexer}
}
