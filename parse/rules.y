%{
package parse

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/cptaffe/mailrules/rules"
)
%}

%union{
    Value      string
    Values     []string
    Rules      []rules.Rule
    Rule       rules.Rule
    MoveRule   *rules.MoveRule
    FlagRule   *rules.FlagRule
    UnflagRule *rules.UnflagRule
    StreamRule *rules.StreamRule
    Predicate  rules.Predicate
}

%left AND OR
%right NOT

%type <Rules> rules
%type <Rule> rule
%type <MoveRule> move
%type <FlagRule> flag
%type <UnflagRule> unflag
%type <StreamRule> stream
%type <Predicate> condition comparison
%type <Values> list
%type <Value> string

%token <Value> IDENTIFIER QUOTE TILDE EQUALS THEN SEMICOLON IF MOVE FLAG UNFLAG STREAM LPAREN RPAREN

%%
start: rules
    { yylex.(*Parser).result = $1 }

rules: rule SEMICOLON
    { $$ = append($$, $1) }
    | rules rule SEMICOLON
    { $$ = append($$, $2) }

rule: IF condition THEN move
    {
        $4.Predicate = $2
        $$ = $4
    }
    | IF condition THEN flag
    {
        $4.Predicate = $2
        $$ = $4
    }
    | IF condition THEN unflag
    {
        $4.Predicate = $2
        $$ = $4
    }
    | IF condition THEN stream
    {
        $4.Predicate = $2
        $$ = $4
    }

condition: comparison
    { $$ = $1 }
    | condition AND condition
    { $$ = &rules.AndPredicate{Left: $1, Right: $3} }
    | condition OR condition
    { $$ = &rules.OrPredicate{Left: $1, Right: $3} }
    | NOT condition
    { $$ = &rules.NotPredicate{Predicate: $2} }
    | LPAREN condition RPAREN
    { $$ = $2 }

comparison:
    IDENTIFIER TILDE string
    {
        rexp, err := regexp.Compile($3)
        if err != nil {
            yylex.Error(fmt.Sprintf("malformed regex '%s' in predicate: %v", $3, err))
            return -1
        }
        $$, err = rules.NewFieldPredicate($1, rexp)
        if err != nil {
            yylex.Error(err.Error())
            return -1
        }
    }
    | IDENTIFIER EQUALS string
    {
        predicate, err := rules.NewFieldPredicate($1, rules.StringEqualsPredicate($3))
        if err != nil {
            yylex.Error(err.Error())
            return -1
        }
        $$ = predicate
    }

move: MOVE string
    { $$ = rules.NewMoveRule(nil, $2) }

flag: FLAG
    { $$ = rules.NewFlagRule(nil, "") }
    | FLAG string
    { $$ = rules.NewFlagRule(nil, $2) }

unflag: UNFLAG
    { $$ = rules.NewUnflagRule(nil, "") }
    | UNFLAG string
    { $$ = rules.NewUnflagRule(nil, $2) }

stream: STREAM string
    { $$ = rules.NewStreamRule(nil, $2) }

list: string
    { $$ = append($$, $1) }
    | IDENTIFIER
    { $$ = append($$, $1) }
    | list string
    { $$ = append($1, $2) }
    | list IDENTIFIER
    { $$ = append($1, $2) }

string: QUOTE
    { $$ = strings.ReplaceAll(strings.ReplaceAll($1[1:len($1)-1], "\\\"", "\""), "\\\\", "\\") }
