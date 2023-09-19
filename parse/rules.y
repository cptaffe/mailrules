%{
package parse

import (
    "fmt"
    "regexp"

    "github.com/cptaffe/mailrules/rules"
)
%}

%union{
    Value      string
    Rules      []rules.Rule
    Rule       rules.Rule
    MoveRule   *rules.MoveRule
    FlagRule   *rules.FlagRule
    UnflagRule *rules.UnflagRule
    Predicate  rules.Predicate
}

%left AND OR
%right NOT

%type <Rules> rules
%type <Rule> rule
%type <MoveRule> move
%type <FlagRule> flag
%type <UnflagRule> unflag
%type <Predicate> condition comparison

%token <Value> IDENTIFIER QUOTE TILDE EQUALS THEN SEMICOLON IF MOVE FLAG UNFLAG LPAREN RPAREN

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
    IDENTIFIER TILDE QUOTE
    {
        rexp, err := regexp.Compile($3[1:len($3)-1])
        if err != nil {
            yylex.Error(fmt.Sprintf("malformed regex '%s' in predicate: %v", $3[1:len($3)-1], err))
            return -1
        }
        $$, err = rules.NewFieldPredicate($1, rexp)
        if err != nil {
            yylex.Error(err.Error())
            return -1
        }
    }
    | IDENTIFIER EQUALS QUOTE
    {
        predicate, err := rules.NewFieldPredicate($1, rules.StringEqualsPredicate($3[1:len($3)-1]))
        if err != nil {
            yylex.Error(err.Error())
            return -1
        }
        $$ = predicate
    }

move: MOVE QUOTE
    { $$ = rules.NewMoveRule(nil, $2[1:len($2)-1]) }

flag: FLAG
    { $$ = rules.NewFlagRule(nil, "") }
    | FLAG QUOTE
    { $$ = rules.NewFlagRule(nil, $2[1:len($2)-1]) }

unflag: UNFLAG
    { $$ = rules.NewUnflagRule(nil, "") }
    | UNFLAG QUOTE
    { $$ = rules.NewUnflagRule(nil, $2[1:len($2)-1]) }
